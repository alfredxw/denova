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
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return LANHTTPHost
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			return ip.String()
		}
	}
	return LANHTTPHost
}

// PrepareUserSettingsForWrite normalizes remote-access credentials before
// replacing the user-level settings file. Blank password input preserves the
// existing password hash.
func PrepareUserSettingsForWrite(existing, incoming Settings) (Settings, error) {
	out := incoming
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
