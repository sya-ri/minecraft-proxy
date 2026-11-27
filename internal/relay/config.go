package relay

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/oauth2"
)

const credentialService = "minecraft-proxy"

var (
	projectPattern  = regexp.MustCompile("^[a-z][a-z0-9-]{4,28}[a-z0-9]$")
	zonePattern     = regexp.MustCompile("^asia-northeast1-[abc]$")
	resourcePattern = regexp.MustCompile("^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$")
	domainPattern   = regexp.MustCompile("^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$")
)

type Config struct {
	Project            string
	Zone               string
	Network            string
	Domain             string
	CloudflareZone     string
	TCPPort            int
	UDPPort            int
	Hours              int
	GoogleClientID     string
	CloudflareClientID string
}

func DefaultConfig(googleID, cloudflareID string) Config {
	return Config{Zone: "asia-northeast1-a", Network: "default", TCPPort: 25565, Hours: 4, GoogleClientID: googleID, CloudflareClientID: cloudflareID}
}

func (c *Config) normalize() {
	c.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.Domain)), ".")
	c.CloudflareZone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.CloudflareZone)), ".")
	c.Project = strings.TrimSpace(c.Project)
	c.GoogleClientID = strings.TrimSpace(c.GoogleClientID)
	c.CloudflareClientID = strings.TrimSpace(c.CloudflareClientID)
}

func (c Config) validate() error {
	if !projectPattern.MatchString(c.Project) || !zonePattern.MatchString(c.Zone) || !resourcePattern.MatchString(c.Network) {
		return errors.New("Enter a valid Google Cloud project ID, Tokyo zone, and VPC network name.")
	}
	if len(c.Domain) > 253 || !domainPattern.MatchString(c.Domain) || !domainPattern.MatchString(c.CloudflareZone) ||
		(c.Domain != c.CloudflareZone && !strings.HasSuffix(c.Domain, "."+c.CloudflareZone)) {
		return errors.New("Enter a hostname and its Cloudflare zone.")
	}
	if c.TCPPort < 1 || c.TCPPort > 65535 || c.UDPPort < 0 || c.UDPPort > 65535 || c.Hours < 1 || c.Hours > 12 {
		return errors.New("Check the port numbers and duration.")
	}
	return nil
}

func (c Config) name() string {
	sum := sha256.Sum256([]byte(c.Domain))
	return fmt.Sprintf("minecraft-proxy-%x", sum[:10])
}

func (c Config) sessionKey() string {
	return strings.Join([]string{"relay", c.Project, c.Zone, c.name()}, "/")
}

func loadConfig(path string, defaults Config) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	err = json.Unmarshal(data, &defaults)
	return defaults, err
}

func saveConfig(path string, config Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func (c Config) oauth(provider string) (*oauth2.Config, string, error) {
	cfg := &oauth2.Config{}
	switch provider {
	case "google":
		cfg.ClientID = c.GoogleClientID
		cfg.Endpoint = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams}
		cfg.Scopes = []string{"https://www.googleapis.com/auth/compute"}
	case "cloudflare":
		cfg.ClientID = c.CloudflareClientID
		cfg.Endpoint = oauth2.Endpoint{AuthURL: "https://dash.cloudflare.com/oauth2/auth", TokenURL: "https://dash.cloudflare.com/oauth2/token", AuthStyle: oauth2.AuthStyleInParams}
		cfg.Scopes = []string{"zone.read", "dns.write", "offline_access"}
	default:
		return nil, "", errors.New("unsupported sign-in provider")
	}
	if cfg.ClientID == "" {
		return nil, "", fmt.Errorf("Enter the %s OAuth client ID.", provider)
	}
	key := sha256.Sum256([]byte(cfg.ClientID))
	return cfg, fmt.Sprintf("%s-%x", provider, key[:10]), nil
}
