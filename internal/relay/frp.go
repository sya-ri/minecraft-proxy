package relay

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const frpVersion = "0.72.0"

//go:embed all:bin
var frpFiles embed.FS

type identity struct {
	RequestID   string
	Nonce       string
	Token       string
	Certificate []byte
	PrivateKey  []byte
	Deadline    time.Time
	InstanceID  string
	PublicIP    string
	TCPPort     int
	UDPPort     int
}

func newIdentity(config Config) (identity, error) {
	session := identity{
		RequestID: requestID(), Nonce: randomHex(16), Token: randomHex(32),
		Deadline: time.Now().UTC().Add(time.Duration(config.Hours) * time.Hour).Truncate(time.Second),
		TCPPort:  config.TCPPort, UDPPort: config.UDPPort,
	}
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return identity{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return identity{}, err
	}
	certificate := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: config.Domain},
		DNSNames: []string{config.Domain}, NotBefore: time.Now().Add(-5 * time.Minute), NotAfter: time.Now().Add(121 * 24 * time.Hour),
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &private.PublicKey, private)
	if err != nil {
		return identity{}, err
	}
	key, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		return identity{}, err
	}
	session.Certificate = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	session.PrivateKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key})
	return session, nil
}

func serverStartup(session identity) string {
	archive := "frp_" + frpVersion + "_linux_amd64.tar.gz"
	const checksum = "daf719b32ca2778770bb92955e231a07414d754378b9af328aa2642862479804"
	ports := "[{single = 25565}]"
	if session.UDPPort != 0 && session.UDPPort != 25565 {
		ports = fmt.Sprintf("[{single = 25565}, {single = %d}]", session.UDPPort)
	}
	return fmt.Sprintf("#!/bin/sh\nset -eu\numask 077\n"+
		"install -d -m 700 /opt/minecraft-proxy\ncd /opt/minecraft-proxy\n"+
		"if ! command -v curl >/dev/null; then apt-get update && apt-get install -y --no-install-recommends curl ca-certificates; fi\n"+
		"curl --fail --silent --show-error --location --retry 2 --max-time 90 https://github.com/fatedier/frp/releases/download/v%s/%s -o frp.tar.gz\n"+
		"printf '%%s  frp.tar.gz\\n' '%s' | sha256sum -c -\n"+
		"tar -xzf frp.tar.gz --strip-components=1 frp_%s_linux_amd64/frps\n"+
		"printf '%%s' '%s' | base64 -d > server.crt\n"+
		"printf '%%s' '%s' | base64 -d > server.key\n"+
		"cat > frps.toml <<'FRP_CONFIG'\n"+
		"bindPort = 7000\nallowPorts = %s\nauth.token = %q\ntransport.tls.force = true\n"+
		"transport.tls.certFile = \"/opt/minecraft-proxy/server.crt\"\ntransport.tls.keyFile = \"/opt/minecraft-proxy/server.key\"\n"+
		"FRP_CONFIG\n"+
		"cat > /etc/systemd/system/minecraft-proxy.service <<'FRP_UNIT'\n"+
		"[Unit]\nAfter=network-online.target\n[Service]\nWorkingDirectory=/opt/minecraft-proxy\n"+
		"ExecStart=/opt/minecraft-proxy/frps -c /opt/minecraft-proxy/frps.toml\n"+
		"Restart=on-failure\nNoNewPrivileges=true\n[Install]\nWantedBy=multi-user.target\nFRP_UNIT\n"+
		"systemctl daemon-reload\nsystemctl enable --now minecraft-proxy.service\n",
		frpVersion, archive, checksum, frpVersion, base64.StdEncoding.EncodeToString(session.Certificate),
		base64.StdEncoding.EncodeToString(session.PrivateKey), ports, session.Token)
}

func frpcConfig(config Config, session identity, ip, certPath string) string {
	var result strings.Builder
	fmt.Fprintf(&result, "serverAddr = %q\nserverPort = 7000\nloginFailExit = false\nauth.token = %q\n", ip, session.Token)
	fmt.Fprintf(&result, "transport.tls.enable = true\ntransport.tls.trustedCaFile = %q\ntransport.tls.serverName = %q\n", filepath.ToSlash(certPath), config.Domain)
	fmt.Fprintf(&result, "\n[[proxies]]\nname = \"minecraft-java\"\ntype = \"tcp\"\nlocalIP = \"127.0.0.1\"\nlocalPort = %d\nremotePort = 25565\n", config.TCPPort)
	if config.UDPPort != 0 {
		fmt.Fprintf(&result, "\n[[proxies]]\nname = \"udp-service\"\ntype = \"udp\"\nlocalIP = \"127.0.0.1\"\nlocalPort = %d\nremotePort = %d\n", config.UDPPort, config.UDPPort)
	}
	return result.String()
}

type tunnel struct {
	cmd      *exec.Cmd
	done     chan struct{}
	udpReady chan struct{}
	err      error
}

func launchFrpc(config Config, session identity, ip string) (*tunnel, error) {
	name := "frpc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, err := frpFiles.ReadFile("bin/" + name)
	if err != nil {
		return nil, errors.New("This build does not contain frpc for your OS.")
	}
	dir, err := os.MkdirTemp("", "minecraft-proxy-frpc-")
	if err != nil {
		return nil, err
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(dir)
		}
	}()
	certPath := filepath.Join(dir, "server.crt")
	configPath := filepath.Join(dir, "frpc.toml")
	executable := filepath.Join(dir, name)
	for _, file := range []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{certPath, session.Certificate, 0600},
		{configPath, []byte(frpcConfig(config, session, ip, certPath)), 0600},
		{executable, binary, 0700},
	} {
		if err := os.WriteFile(file.path, file.data, file.mode); err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(executable, "-c", configPath)
	hideConsole(cmd)
	logs, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	t := &tunnel{cmd: cmd, done: make(chan struct{}), udpReady: make(chan struct{}, 1)}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	started = true
	go func() {
		lines := bufio.NewScanner(logs)
		for lines.Scan() {
			line := lines.Text()
			if strings.Contains(line, "[udp-service]") && strings.Contains(line, "start proxy success") {
				select {
				case t.udpReady <- struct{}{}:
				default:
				}
			}
		}
		_, _ = io.Copy(io.Discard, logs)
		t.err = cmd.Wait()
		_ = os.RemoveAll(dir)
		close(t.done)
	}()
	return t, nil
}

func (t *tunnel) stop() {
	if t == nil {
		return
	}
	_ = t.cmd.Process.Kill()
	select {
	case <-t.done:
	case <-time.After(3 * time.Second):
	}
}

func (t *tunnel) waitReady(ctx context.Context, config Config, ip string) error {
	udpReady := config.UDPPort == 0
	address := net.JoinHostPort(ip, strconv.Itoa(25565))
	for {
		select {
		case <-t.udpReady:
			udpReady = true
		case <-t.done:
			return fmt.Errorf("The relay client exited: %v", t.err)
		default:
		}
		if udpReady && pingMinecraft(ctx, address, config.Domain, 25565) == nil {
			return nil
		}
		if err := pause(ctx, 2*time.Second); err != nil {
			return errors.New("The public port did not return a Minecraft status. Check the GCE firewall and local server.")
		}
	}
}
