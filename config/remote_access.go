package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	LocalHTTPHost = "127.0.0.1"
	LANHTTPHost   = "0.0.0.0"
)

var (
	ErrRemoteAccessUsernameRequired = errors.New("remote access username required")
	ErrRemoteAccessPasswordRequired = errors.New("remote access password required")
)

// RemoteAccessConfig is the runtime subset needed by the HTTP access gate.
type RemoteAccessConfig struct {
	DataDir        string // Runtime-only root for browser credentials.
	AllowLANAccess bool
	Username       string
	PasswordHash   string
}

func HTTPListenHost(allowLANAccess bool) string {
	if allowLANAccess {
		return LANHTTPHost
	}
	return LocalHTTPHost
}

func HTTPURL(host string, port int) string {
	if port <= 0 {
		port = 8080
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

func LocalHTTPURL(port int) string {
	return HTTPURL("localhost", port)
}

func LANHTTPURL(port int) string {
	return HTTPURL(LANAddress(), port)
}

func LANAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return LANHTTPHost
	}
	var addrs []net.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if addresses, err := iface.Addrs(); err == nil {
			addrs = append(addrs, addresses...)
		}
	}
	return selectLANAddress(addrs)
}

// Prefer private IPv4 addresses for device-to-device access. Benchmark networks
// (198.18.0.0/15) are also used by local proxy tunnels and must not be advertised.
func selectLANAddress(addrs []net.Addr) string {
	fallback := LANHTTPHost
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil || !ip.IsGlobalUnicast() || (ip[0] == 198 && (ip[1] == 18 || ip[1] == 19)) {
			continue
		}
		if ip.IsPrivate() {
			return ip.String()
		}
		if fallback == LANHTTPHost {
			fallback = ip.String()
		}
	}
	return fallback
}

// PrepareUserSettingsForWrite normalizes remote-access credentials before
// replacing the user-level settings file. Blank password input preserves the
// existing password hash.
func PrepareUserSettingsForWrite(existing, incoming Settings) (Settings, error) {
	out := preserveTerminalCommandRegistryPresence(incoming)
	if err := validateSettingsCheckpointGuidance(out); err != nil {
		return Settings{}, err
	}
	out.AgentQuickPrompts = normalizeAgentQuickPrompts(out.AgentQuickPrompts)
	out.AgentApprovalRules = NormalizeAgentApprovalRules(out.AgentApprovalRules)
	if err := ValidateAgentApprovalRules(out.AgentApprovalRules); err != nil {
		return Settings{}, err
	}
	if err := validateTerminalCommands(out.TerminalCommands); err != nil {
		return Settings{}, err
	}
	if err := validateAgentQuickPrompts(out.AgentQuickPrompts); err != nil {
		return Settings{}, err
	}
	out.RemoteAccessUsername = strings.TrimSpace(out.RemoteAccessUsername)
	if out.RemoteAccessPassword != "" {
		hash, err := HashRemoteAccessPassword(out.RemoteAccessPassword)
		if err != nil {
			return Settings{}, err
		}
		out.RemoteAccessPasswordHash = hash
	} else if out.RemoteAccessPasswordHash == "" {
		out.RemoteAccessPasswordHash = existing.RemoteAccessPasswordHash
	}
	out.RemoteAccessPassword = ""
	out.RemoteAccessPasswordSet = out.RemoteAccessPasswordHash != ""

	if out.AllowLANAccess != nil && *out.AllowLANAccess {
		if out.RemoteAccessUsername == "" {
			return Settings{}, ErrRemoteAccessUsernameRequired
		}
		if out.RemoteAccessPasswordHash == "" {
			return Settings{}, ErrRemoteAccessPasswordRequired
		}
	}
	return out, nil
}

func HashRemoteAccessPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", ErrRemoteAccessPasswordRequired
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成远程访问密码哈希失败: %w", err)
	}
	return string(hash), nil
}

func CheckRemoteAccessPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
