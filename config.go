package main

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
)

// Config holds the sudo (owner-level) and approved (limited) user lists.
// Stored as plain JSON on disk — no database needed.
type Config struct {
	mu sync.Mutex `json:"-"`

	// SudoIDs are full-access users. The self-account (you) is always
	// treated as sudo even if not listed here — see IsSudo.
	SudoIDs []int64 `json:"sudo_ids"`

	// ApprovedIDs are users allowed to run general (non-sudo) commands.
	ApprovedIDs []int64 `json:"approved_ids"`

	// OwnerID is the userbot account's own Telegram user ID, filled in
	// automatically on first login. Always treated as sudo.
	OwnerID int64 `json:"owner_id"`

	// Warnings tracks warn counts per user ID, keyed as a string because
	// JSON object keys must be strings.
	Warnings map[string]int `json:"warnings"`

	path string
}

const configPath = "users.json"

// LoadConfig reads users.json, creating a fresh empty one if it doesn't exist.
func LoadConfig() (*Config, error) {
	cfg := &Config{path: configPath, Warnings: map[string]int{}}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return cfg, cfg.Save()
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.path = configPath
	if cfg.Warnings == nil {
		cfg.Warnings = map[string]int{}
	}
	return cfg, nil
}

// Save persists the config to disk.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o600)
}

// IsSudo reports whether id has full (owner-level) access.
func (c *Config) IsSudo(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == c.OwnerID {
		return true
	}
	for _, sid := range c.SudoIDs {
		if sid == id {
			return true
		}
	}
	return false
}

// IsApproved reports whether id has been approved for general commands.
// Sudo users are implicitly approved too.
func (c *Config) IsApproved(id int64) bool {
	if c.IsSudo(id) {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, aid := range c.ApprovedIDs {
		if aid == id {
			return true
		}
	}
	return false
}

// Approve adds id to the approved list. Returns false if already present.
func (c *Config) Approve(id int64) bool {
	if c.IsApproved(id) {
		return false
	}
	c.mu.Lock()
	c.ApprovedIDs = append(c.ApprovedIDs, id)
	c.mu.Unlock()
	_ = c.Save()
	return true
}

// Unapprove removes id from the approved list. Returns false if not present.
func (c *Config) Unapprove(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, aid := range c.ApprovedIDs {
		if aid == id {
			c.ApprovedIDs = append(c.ApprovedIDs[:i], c.ApprovedIDs[i+1:]...)
			_ = c.saveLocked()
			return true
		}
	}
	return false
}

// AddSudo grants full sudo access to id. Only ever call this in response
// to a command from an *existing* sudo user.
func (c *Config) AddSudo(id int64) bool {
	if c.IsSudo(id) {
		return false
	}
	c.mu.Lock()
	c.SudoIDs = append(c.SudoIDs, id)
	c.mu.Unlock()
	_ = c.Save()
	return true
}

// RmSudo revokes sudo from id. It refuses to remove the original owner —
// that would permanently lock everyone out.
func (c *Config) RmSudo(id int64) bool {
	if id == c.OwnerID {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, sid := range c.SudoIDs {
		if sid == id {
			c.SudoIDs = append(c.SudoIDs[:i], c.SudoIDs[i+1:]...)
			_ = c.saveLocked()
			return true
		}
	}
	return false
}

// Warn increments id's warning count and returns the new total.
func (c *Config) Warn(id int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strconv.FormatInt(id, 10)
	c.Warnings[key]++
	n := c.Warnings[key]
	_ = c.saveLocked()
	return n
}

// WarnCount returns id's current warning count.
func (c *Config) WarnCount(id int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Warnings[strconv.FormatInt(id, 10)]
}

// ResetWarn clears id's warning count back to zero.
func (c *Config) ResetWarn(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Warnings, strconv.FormatInt(id, 10))
	_ = c.saveLocked()
}

// saveLocked assumes c.mu is already held.
func (c *Config) saveLocked() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o600)
}
