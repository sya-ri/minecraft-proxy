package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

type State struct {
	Config          Config
	Phase           string
	Problem         string
	Deadline        time.Time
	GoogleReady     bool
	CloudflareReady bool
}

type Service struct {
	mu         sync.Mutex
	operation  sync.Mutex
	state      State
	configPath string
	process    *tunnel
}

func newService(path string, defaults Config) (*Service, error) {
	config, err := loadConfig(path, defaults)
	if err != nil {
		return nil, err
	}
	s := &Service{configPath: path, state: State{Config: config, Phase: "idle"}}
	s.refreshLogins()
	if config.validate() == nil {
		var session identity
		if err := readCredential(config.sessionKey(), &session); err == nil {
			s.state.Phase = "recoverable"
			s.state.Deadline = session.Deadline
		}
	}
	return s, nil
}

func (s *Service) GetState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Service) setPhase(phase string, deadline time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Phase = phase
	s.state.Deadline = deadline
	s.state.Problem = ""
	if err != nil {
		s.state.Problem = err.Error()
	}
}

func (s *Service) refreshLogins() {
	s.mu.Lock()
	defer s.mu.Unlock()
	ready := func(provider string) bool {
		_, key, err := s.state.Config.oauth(provider)
		if err != nil {
			return false
		}
		var token oauth2.Token
		return readCredential(key, &token) == nil && token.RefreshToken != ""
	}
	s.state.GoogleReady = ready("google")
	s.state.CloudflareReady = ready("cloudflare")
}

func (s *Service) SaveConfig(config Config) error {
	if !s.operation.TryLock() {
		return errors.New("An operation is already in progress.")
	}
	defer s.operation.Unlock()
	state := s.GetState()
	if state.Phase == "running" || state.Phase == "recoverable" {
		return errors.New("Stop the relay before editing settings.")
	}
	config.normalize()
	if err := config.validate(); err != nil {
		return err
	}
	if err := saveConfig(s.configPath, config); err != nil {
		return err
	}
	s.mu.Lock()
	s.state.Config = config
	s.mu.Unlock()
	s.refreshLogins()
	return nil
}

func (s *Service) LogIn(ctx context.Context, provider string) error {
	if !s.operation.TryLock() {
		return errors.New("An operation is already in progress.")
	}
	defer s.operation.Unlock()
	config := s.GetState().Config
	cfg, key, err := config.oauth(provider)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := authorize(ctx, cfg, provider, key); err != nil {
		return err
	}
	s.refreshLogins()
	return nil
}

func (s *Service) Start(ctx context.Context, replaceDNS bool) (err error) {
	if !s.operation.TryLock() {
		return errors.New("An operation is already in progress.")
	}
	defer s.operation.Unlock()
	state := s.GetState()
	if state.Phase == "running" {
		return nil
	}
	config := state.Config
	if err := config.validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var session identity
	var started *tunnel
	pending := false
	created := false
	var gce compute
	var dns cloudflare
	var zoneID string
	s.setPhase("starting", state.Deadline, nil)
	defer func() {
		if err == nil {
			return
		}
		started.stop()
		if created {
			var cleanupErr error
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 50*time.Second)
			cleanupErr = s.removeRelay(cleanupCtx, config, session, gce.client, dns.client, zoneID)
			cleanupCancel()
			if cleanupErr == nil {
				pending = false
			} else {
				err = errors.Join(err, fmt.Errorf("cleanup: %w", cleanupErr))
			}
		}
		if pending {
			s.setPhase("recoverable", session.Deadline, err)
		} else {
			s.setPhase("idle", time.Time{}, err)
		}
	}()
	key := config.sessionKey()
	if stored := readCredential(key, &session); stored == nil {
		pending = true
	} else if !errors.Is(stored, keyring.ErrNotFound) {
		return fmt.Errorf("open the operating system credential store: %w", stored)
	}
	if err := pingMinecraft(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(config.TCPPort)), "localhost", config.TCPPort); err != nil {
		return fmt.Errorf("Check your local Minecraft Java server on port %d: %w", config.TCPPort, err)
	}
	google, err := authenticatedClient(context.Background(), config, "google")
	if err != nil {
		return err
	}
	cf, err := authenticatedClient(context.Background(), config, "cloudflare")
	if err != nil {
		return err
	}
	gce = compute{google, config}
	dns = cloudflare{cf, config}
	zoneID, err = dns.zone(ctx)
	if err != nil {
		return err
	}

	vm, err := gce.lookup(ctx)
	if err != nil {
		return err
	}
	if vm != nil && !gce.owns(vm, session) {
		return errors.New("A relay with this name already exists. Check it on the computer that started it.")
	}
	if vm == nil && pending && (session.InstanceID != "" || time.Until(session.Deadline) < time.Minute) {
		if err := dns.unpublish(ctx, zoneID, session); err != nil {
			return err
		}
		if err := keyring.Delete(credentialService, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return err
		}
		session = identity{}
		pending = false
	}
	if pending && (session.TCPPort != config.TCPPort || session.UDPPort != config.UDPPort) {
		return errors.New("The existing relay uses different ports. Stop it first.")
	}
	if _, err := dns.ensureAvailable(ctx, cfEndpoint+"/zones/"+zoneID+"/dns_records", session, replaceDNS); err != nil {
		return err
	}
	if vm == nil {
		if !pending {
			session, err = newIdentity(config)
			if err != nil {
				return err
			}
			if err := writeCredential(key, session); err != nil {
				return err
			}
			pending = true
		}
		err := gce.create(ctx, session)
		if err != nil && !isHTTP(err, 409) {
			// The server may have accepted the request. Retain the session for recovery.
			return fmt.Errorf("Could not confirm relay creation: %w", err)
		}
		created = err == nil
	}
	vm, err = waitInstance(ctx, gce, session)
	if err != nil {
		return err
	}
	if session.InstanceID != "" && vm.ID != session.InstanceID {
		return errors.New("The VM ID does not match the saved session.")
	}
	session.InstanceID, session.PublicIP = vm.ID, vm.ip()
	if deadline := vm.expiry(); !deadline.IsZero() {
		session.Deadline = deadline
	}
	if err := writeCredential(key, session); err != nil {
		return err
	}
	started, err = launchFrpc(config, session, session.PublicIP)
	if err != nil {
		return err
	}
	if err := started.waitReady(ctx, config, session.PublicIP); err != nil {
		return err
	}
	if err := dns.publish(ctx, zoneID, session, session.PublicIP, replaceDNS); err != nil {
		return fmt.Errorf("Update Cloudflare DNS: %w", err)
	}

	s.process = started
	s.setPhase("running", session.Deadline, nil)
	go s.watch(started, session.Deadline)
	return nil
}

func waitInstance(ctx context.Context, gce compute, session identity) (*instance, error) {
	for {
		vm, err := gce.lookup(ctx)
		if err != nil {
			return nil, err
		}
		if vm != nil {
			if !gce.owns(vm, session) {
				return nil, errors.New("Another session created the relay with this name.")
			}
			if vm.Status == "RUNNING" && vm.ip() != "" {
				return vm, nil
			}
			if vm.Status == "TERMINATED" || vm.Status == "SUSPENDED" {
				return nil, errors.New("The existing VM is stopped. Delete it before reconnecting.")
			}
		}
		if err := pause(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
}

func (s *Service) watch(started *tunnel, deadline time.Time) {
	for {
		timer := time.NewTimer(time.Until(deadline))
		exited := false
		select {
		case <-timer.C:
		case <-started.done:
			exited = true
		}
		timer.Stop()

		s.operation.Lock()
		if s.process != started {
			s.operation.Unlock()
			return
		}
		deadline = s.GetState().Deadline
		if !exited && time.Now().Before(deadline) {
			s.operation.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		_ = s.stop(ctx)
		cancel()
		s.operation.Unlock()
		return
	}
}

const extension = 4 * time.Hour

func (s *Service) Extend(ctx context.Context) error {
	if !s.operation.TryLock() {
		return errors.New("An operation is already in progress.")
	}
	defer s.operation.Unlock()
	state := s.GetState()
	if state.Phase != "running" {
		return errors.New("Start the relay before extending it.")
	}
	config := state.Config
	var session identity
	if err := readCredential(config.sessionKey(), &session); err != nil {
		return err
	}
	client, err := authenticatedClient(context.Background(), config, "google")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	deadline, err := (compute{client, config}).extend(ctx, session)
	if err != nil {
		return err
	}
	s.setPhase("running", deadline, nil)
	session.Deadline = deadline
	return writeCredential(config.sessionKey(), session)
}

func (s *Service) Stop(ctx context.Context) error {
	if !s.operation.TryLock() {
		return errors.New("An operation is already in progress.")
	}
	defer s.operation.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.stop(ctx)
}

func (s *Service) stop(ctx context.Context) error {
	state := s.GetState()
	config := state.Config
	s.process.stop()
	s.process = nil
	var session identity
	err := readCredential(config.sessionKey(), &session)
	if errors.Is(err, keyring.ErrNotFound) {
		s.setPhase("idle", time.Time{}, nil)
		return nil
	}
	if err != nil {
		s.setPhase("recoverable", state.Deadline, err)
		return err
	}
	s.setPhase("stopping", session.Deadline, nil)
	google, googleErr := authenticatedClient(context.Background(), config, "google")
	cf, cfErr := authenticatedClient(context.Background(), config, "cloudflare")
	err = errors.Join(googleErr, cfErr, s.removeRelay(ctx, config, session, google, cf, ""))
	if err != nil {
		s.setPhase("recoverable", session.Deadline, err)
		return err
	}
	s.setPhase("idle", time.Time{}, nil)
	return nil
}

func (s *Service) removeRelay(ctx context.Context, config Config, session identity, google, cf *http.Client, zoneID string) error {
	var dnsErr, vmErr error
	if cf != nil {
		dns := cloudflare{cf, config}
		dnsCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if zoneID == "" {
			zoneID, dnsErr = dns.zone(dnsCtx)
		}
		if dnsErr == nil {
			dnsErr = dns.unpublish(dnsCtx, zoneID, session)
		}
		cancel()
	}
	if google != nil {
		vmErr = (compute{google, config}).delete(ctx, session)
	}
	if err := errors.Join(dnsErr, vmErr); err != nil {
		return err
	}
	if google != nil && cf != nil {
		if err := keyring.Delete(credentialService, config.sessionKey()); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) closeApp() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state := s.GetState()
	if state.Phase != "idle" {
		return s.Stop(ctx)
	}
	return nil
}
