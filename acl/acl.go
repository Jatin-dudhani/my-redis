package acl

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Categories of Redis commands
var Categories = map[string][]string{
	"read": {
		"GET", "MGET", "HGET", "HGETALL", "HEXISTS", "LRANGE",
		"SMEMBERS", "SISMEMBER", "ZRANGE", "ZSCORE", "XRANGE",
		"XLEN", "TYPE", "KEYS", "DBSIZE", "EXISTS", "TTL",
		"GETBIT", "BITCOUNT", "BITPOS", "PFCOUNT", "GEODIST",
		"GEOPOS", "GEOSEARCH", "GEOHASH",
	},
	"write": {
		"SET", "DEL", "EXPIRE", "LPUSH", "RPUSH", "LPOP", "RPOP",
		"BLPOP", "BRPOP", "SADD", "SREM", "HSET", "HDEL", "ZADD",
		"ZREM", "XADD", "SETBIT", "BITOP", "PFADD", "PFMERGE", "GEOADD",
	},
	"admin": {
		"CONFIG", "SAVE", "BGSAVE", "BGREWRITEAOF", "FLUSHDB",
		"MAXMEMORY", "REPLICAOF", "CLUSTER", "ACL",
	},
	"connection": {
		"PING", "AUTH", "INFO", "QUIT", "COMMAND",
	},
	"scripting": {
		"EVAL", "EVALSHA", "SCRIPT",
	},
}

// User represents an ACL user identity and permission model.
type User struct {
	Username    string
	Enabled     bool
	NoPass      bool
	Passwords   map[string]bool // SHA-256 hex hashed passwords
	AllCommands bool
	AllowedCmds map[string]bool
	DeniedCmds  map[string]bool
	AllKeys     bool
	KeyPatterns []string
}

// Manager controls the ACL registry.
type Manager struct {
	mu    sync.RWMutex
	users map[string]*User
}

// HashPassword computes SHA-256 digest of cleartext password.
func HashPassword(pwd string) string {
	h := sha256.Sum256([]byte(pwd))
	return hex.EncodeToString(h[:])
}

// NewManager initializes the ACL manager with a permissive default user.
func NewManager() *Manager {
	m := &Manager{
		users: make(map[string]*User),
	}
	defaultUser := &User{
		Username:    "default",
		Enabled:     true,
		NoPass:      true,
		Passwords:   make(map[string]bool),
		AllCommands: true,
		AllowedCmds: make(map[string]bool),
		DeniedCmds:  make(map[string]bool),
		AllKeys:     true,
		KeyPatterns: nil,
	}
	m.users["default"] = defaultUser
	return m
}

// SetUser applies ACL rules for a username.
func (m *Manager) SetUser(username string, rules []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, exists := m.users[username]
	if !exists {
		u = &User{
			Username:    username,
			Enabled:     false,
			NoPass:      false,
			Passwords:   make(map[string]bool),
			AllCommands: false,
			AllowedCmds: make(map[string]bool),
			DeniedCmds:  make(map[string]bool),
			AllKeys:     false,
			KeyPatterns: nil,
		}
		m.users[username] = u
	}

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		switch {
		case rule == "on":
			u.Enabled = true
		case rule == "off":
			u.Enabled = false
		case rule == "nopass":
			u.NoPass = true
			u.Passwords = make(map[string]bool)
		case strings.HasPrefix(rule, ">"):
			// Cleartext password
			plain := rule[1:]
			hash := HashPassword(plain)
			u.Passwords[hash] = true
			u.NoPass = false
		case strings.HasPrefix(rule, "#"):
			// Pre-hashed password
			hash := strings.ToLower(rule[1:])
			u.Passwords[hash] = true
			u.NoPass = false
		case rule == "+@all":
			u.AllCommands = true
			u.AllowedCmds = make(map[string]bool)
			u.DeniedCmds = make(map[string]bool)
		case rule == "-@all":
			u.AllCommands = false
			u.AllowedCmds = make(map[string]bool)
			u.DeniedCmds = make(map[string]bool)
		case strings.HasPrefix(rule, "+@"):
			cat := strings.ToLower(rule[2:])
			if cmds, ok := Categories[cat]; ok {
				for _, c := range cmds {
					u.AllowedCmds[strings.ToUpper(c)] = true
					delete(u.DeniedCmds, strings.ToUpper(c))
				}
			}
		case strings.HasPrefix(rule, "-@"):
			cat := strings.ToLower(rule[2:])
			if cmds, ok := Categories[cat]; ok {
				for _, c := range cmds {
					delete(u.AllowedCmds, strings.ToUpper(c))
					u.DeniedCmds[strings.ToUpper(c)] = true
				}
			}
		case strings.HasPrefix(rule, "+"):
			cmd := strings.ToUpper(rule[1:])
			u.AllowedCmds[cmd] = true
			delete(u.DeniedCmds, cmd)
		case strings.HasPrefix(rule, "-"):
			cmd := strings.ToUpper(rule[1:])
			delete(u.AllowedCmds, cmd)
			u.DeniedCmds[cmd] = true
		case rule == "~*":
			u.AllKeys = true
			u.KeyPatterns = nil
		case strings.HasPrefix(rule, "~"):
			pattern := rule[1:]
			u.KeyPatterns = append(u.KeyPatterns, pattern)
			u.AllKeys = false
		case rule == "resetpass":
			u.Passwords = make(map[string]bool)
			u.NoPass = false
		}
	}

	return nil
}

// Authenticate verifies username and cleartext password using constant-time comparison.
func (m *Manager) Authenticate(username, password string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, exists := m.users[username]
	if !exists || !u.Enabled {
		return false
	}

	if u.NoPass {
		return true
	}

	candidateHash := HashPassword(password)
	for storedHash := range u.Passwords {
		if subtle.ConstantTimeCompare([]byte(candidateHash), []byte(storedHash)) == 1 {
			return true
		}
	}

	return false
}

// CheckPermission checks if the user has permission to execute cmd on key.
func (m *Manager) CheckPermission(username, cmd, key string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, exists := m.users[username]
	if !exists {
		return false, fmt.Sprintf("NOPERM User '%s' does not exist", username)
	}
	if !u.Enabled {
		return false, fmt.Sprintf("NOPERM User '%s' is disabled", username)
	}

	cmdUpper := strings.ToUpper(cmd)

	// Connection commands are always permitted if authenticated
	if cmdUpper == "AUTH" || cmdUpper == "QUIT" || cmdUpper == "COMMAND" {
		return true, ""
	}

	// Command permission check
	if !u.AllCommands {
		if !u.AllowedCmds[cmdUpper] {
			return false, fmt.Sprintf("NOPERM this user has no permissions to run the '%s' command", cmd)
		}
	} else {
		if u.DeniedCmds[cmdUpper] {
			return false, fmt.Sprintf("NOPERM this user has no permissions to run the '%s' command", cmd)
		}
	}

	// Key permission check
	if key != "" && !u.AllKeys {
		matched := false
		for _, pat := range u.KeyPatterns {
			if ok, _ := filepath.Match(pat, key); ok {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("NOPERM this user has no permissions to access the '%s' key", key)
		}
	}

	return true, ""
}

// ListUsers returns a sorted list of configured usernames.
func (m *Manager) ListUsers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []string
	for name := range m.users {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// DelUser removes specified users (except default).
func (m *Manager) DelUser(usernames ...string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, u := range usernames {
		if u == "default" {
			continue
		}
		if _, ok := m.users[u]; ok {
			delete(m.users, u)
			count++
		}
	}
	return count
}

// DescribeUsers produces Redis-compatible ACL LIST strings.
func (m *Manager) DescribeUsers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lines []string
	for _, u := range m.users {
		var parts []string
		parts = append(parts, fmt.Sprintf("user %s", u.Username))
		if u.Enabled {
			parts = append(parts, "on")
		} else {
			parts = append(parts, "off")
		}

		if u.NoPass {
			parts = append(parts, "nopass")
		} else {
			for h := range u.Passwords {
				parts = append(parts, "#"+h)
			}
		}

		if u.AllKeys {
			parts = append(parts, "~*")
		} else {
			for _, pat := range u.KeyPatterns {
				parts = append(parts, "~"+pat)
			}
		}

		if u.AllCommands {
			parts = append(parts, "+@all")
			for d := range u.DeniedCmds {
				parts = append(parts, "-"+strings.ToLower(d))
			}
		} else {
			parts = append(parts, "-@all")
			for a := range u.AllowedCmds {
				parts = append(parts, "+"+strings.ToLower(a))
			}
		}

		lines = append(lines, strings.Join(parts, " "))
	}
	sort.Strings(lines)
	return lines
}
