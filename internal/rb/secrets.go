package rb

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	encPrefix       = "RB1:"
	masterIterRound = 600000
)

var errLocked = errors.New("locked")

func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

func secretsMode() string {
	return configLoad().Settings.Secrets
}

func keyFile(s Settings) string {
	return s.DataDir + "/.keys/secrets.key"
}

func masterFile() string {
	return SecretsDir + "/master.json"
}

func unlockedKeyFile() string {
	return RunDir + "/secrets/.key"
}

func plainSecretFile(repoID, kind string) string {
	return SecretsDir + "/" + repoID + "." + kind
}

func encSecretFile(repoID, kind string) string {
	return plainSecretFile(repoID, kind) + ".enc"
}

func sshPlainKey(repoID string) string {
	return SSHDir + "/" + repoID
}

func sshEncKey(repoID string) string {
	return SSHDir + "/" + repoID + ".enc"
}

func runSecretsDir() string {
	return RunDir + "/secrets"
}

func dataKey(s Settings) ([]byte, error) {
	switch s.Secrets {
	case "key":
		b, err := os.ReadFile(keyFile(s))
		if err != nil {
			return nil, err
		}
		return decodeKey(b)
	case "master":
		b, err := os.ReadFile(unlockedKeyFile())
		if err != nil {
			return nil, errLocked
		}
		return decodeKey(b)
	}
	return nil, nil
}

func decodeKey(b []byte) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != 32 {
		return nil, errors.New("the key file is damaged")
	}
	return k, nil
}

func seal(key, plain []byte) []byte {
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	out := gcm.Seal(nonce, nonce, plain, nil)
	return []byte(encPrefix + base64.StdEncoding.EncodeToString(out) + "\n")
}

func unseal(key, data []byte) ([]byte, error) {
	s := strings.TrimSpace(string(data))
	if !strings.HasPrefix(s, encPrefix) {
		return nil, errors.New("not an encrypted secret")
	}
	raw, err := base64.StdEncoding.DecodeString(s[len(encPrefix):])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("damaged secret")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}

func secretFile(repoID, kind string) string {
	return plainSecretFile(repoID, kind)
}

func secretRead(repoID, kind string) ([]byte, error) {
	if b, err := os.ReadFile(encSecretFile(repoID, kind)); err == nil {
		key, err := dataKey(configLoad().Settings)
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, errors.New("the secrets are encrypted but encryption is off")
		}
		return unseal(key, b)
	}
	return os.ReadFile(plainSecretFile(repoID, kind))
}

func secretWrite(repoID, kind string, data []byte) bool {
	s := configLoad().Settings
	if s.Secrets == "plain" {
		os.Remove(encSecretFile(repoID, kind))
		return fileWrite(plainSecretFile(repoID, kind), data, 0o600)
	}
	key, err := dataKey(s)
	if err != nil || key == nil {
		return false
	}
	if !fileWrite(encSecretFile(repoID, kind), seal(key, data), 0o600) {
		return false
	}
	os.Remove(plainSecretFile(repoID, kind))
	os.Remove(runSecretsDir() + "/" + repoID + "." + kind)
	return true
}

func secretExists(repoID, kind string) bool {
	for _, f := range []string{encSecretFile(repoID, kind), plainSecretFile(repoID, kind)} {
		if st, err := os.Stat(f); err == nil && st.Size() > 1 {
			return true
		}
	}
	return false
}

func secretRemove(repoID string) {
	for _, kind := range []string{"password", "rest"} {
		os.Remove(plainSecretFile(repoID, kind))
		os.Remove(encSecretFile(repoID, kind))
		os.Remove(runSecretsDir() + "/" + repoID + "." + kind)
	}
	os.Remove(sshPlainKey(repoID))
	os.Remove(sshEncKey(repoID))
	os.Remove(sshPlainKey(repoID) + ".pub")
	os.Remove(runSecretsDir() + "/ssh-" + repoID)
}

func materialize(name string, data []byte) string {
	dir := runSecretsDir()
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	path := dir + "/" + name
	fileWrite(path, data, 0o600)
	return path
}

func secretPasswordFile(repoID string) string {
	if !exists(encSecretFile(repoID, "password")) {
		return plainSecretFile(repoID, "password")
	}
	b, err := secretRead(repoID, "password")
	if err != nil {
		return runSecretsDir() + "/" + repoID + ".password"
	}
	return materialize(repoID+".password", b)
}

func sshKeyPath(repoID string) string {
	enc, err := os.ReadFile(sshEncKey(repoID))
	if err != nil {
		return sshPlainKey(repoID)
	}
	key, err := dataKey(configLoad().Settings)
	if err != nil || key == nil {
		return runSecretsDir() + "/ssh-" + repoID
	}
	b, err := unseal(key, enc)
	if err != nil {
		return runSecretsDir() + "/ssh-" + repoID
	}
	return materialize("ssh-"+repoID, b)
}

func sshKeyExists(repoID string) bool {
	return isFile(sshPlainKey(repoID)) || isFile(sshEncKey(repoID))
}

func sshKeyStore(repoID string, private []byte) bool {
	s := configLoad().Settings
	if s.Secrets == "plain" {
		os.Remove(sshEncKey(repoID))
		return fileWrite(sshPlainKey(repoID), private, 0o600)
	}
	key, err := dataKey(s)
	if err != nil || key == nil {
		return false
	}
	if !fileWrite(sshEncKey(repoID), seal(key, private), 0o600) {
		return false
	}
	os.Remove(sshPlainKey(repoID))
	return true
}

func wipeRunSecrets() {
	dir := runSecretsDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == ".key" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if st, err := os.Stat(p); err == nil {
			zero := make([]byte, st.Size())
			os.WriteFile(p, zero, 0o600)
		}
		os.Remove(p)
	}
}

func secretsLocked() bool {
	s := configLoad().Settings
	if s.Secrets != "master" {
		return false
	}
	_, err := dataKey(s)
	return err != nil
}

func secretsReady(s Settings) string {
	switch s.Secrets {
	case "key":
		if _, err := dataKey(s); err != nil {
			return "The key for the encrypted passwords is missing or unreadable (" + keyFile(s) + "). Is the array started?"
		}
	case "master":
		if _, err := dataKey(s); err != nil {
			return "The passwords are locked. Enter the master password on the plugin page."
		}
	}
	return ""
}

type masterRecord struct {
	Salt       string `json:"salt"`
	Iterations int    `json:"iterations"`
	Key        string `json:"key"`
}

func deriveKEK(password string, salt []byte, iter int) []byte {
	k, _ := pbkdf2.Key(sha256.New, password, salt, iter, 32)
	return k
}

func masterUnlock(password string) error {
	var rec masterRecord
	if !jsonReadInto(masterFile(), &rec) {
		return errors.New("No master password is set.")
	}
	salt, err := base64.StdEncoding.DecodeString(rec.Salt)
	if err != nil {
		return errors.New("The master password record is damaged.")
	}
	kek := deriveKEK(password, salt, rec.Iterations)
	dek, err := unseal(kek, []byte(rec.Key))
	if err != nil || len(dek) != 32 {
		return errors.New("Wrong master password.")
	}
	os.MkdirAll(runSecretsDir(), 0o700)
	if !fileWrite(unlockedKeyFile(), []byte(base64.StdEncoding.EncodeToString(dek)+"\n"), 0o600) {
		return errors.New("The key could not be kept in RAM.")
	}
	return nil
}

func masterLock() {
	wipeRunSecrets()
	if st, err := os.Stat(unlockedKeyFile()); err == nil {
		os.WriteFile(unlockedKeyFile(), make([]byte, st.Size()), 0o600)
	}
	os.Remove(unlockedKeyFile())
}

func masterStore(password string, dek []byte) bool {
	salt := make([]byte, 16)
	rand.Read(salt)
	kek := deriveKEK(password, salt, masterIterRound)
	rec := masterRecord{Salt: base64.StdEncoding.EncodeToString(salt), Iterations: masterIterRound,
		Key: strings.TrimSpace(string(seal(kek, dek)))}
	privateDir(SecretsDir)
	return jsonWrite(masterFile(), rec, 0o600)
}

func masterCheck(password string) bool {
	var rec masterRecord
	if !jsonReadInto(masterFile(), &rec) {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(rec.Salt)
	if err != nil {
		return false
	}
	dek, err := unseal(deriveKEK(password, salt, rec.Iterations), []byte(rec.Key))
	if err != nil {
		return false
	}
	cur, err := os.ReadFile(unlockedKeyFile())
	if err != nil {
		return true
	}
	k, err := decodeKey(cur)
	return err != nil || subtle.ConstantTimeCompare(k, dek) == 1
}

type secretItem struct {
	path  string
	kind  string
	repo  string
	plain []byte
}

func collectSecrets(cfg *Config) ([]secretItem, error) {
	var items []secretItem
	for _, r := range cfg.Repos {
		for _, kind := range []string{"password", "rest"} {
			if !secretExists(r.ID, kind) {
				continue
			}
			b, err := secretRead(r.ID, kind)
			if err != nil {
				return nil, err
			}
			items = append(items, secretItem{kind: kind, repo: r.ID, plain: b})
		}
		if sshKeyExists(r.ID) {
			p := sshKeyPath(r.ID)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			items = append(items, secretItem{kind: "ssh", repo: r.ID, plain: b})
		}
	}
	return items, nil
}

func secretsSwitch(cfg *Config, mode, password string) error {
	cur := cfg.Settings
	if cur.Secrets == mode && mode != "master" {
		return nil
	}
	if cur.Secrets == "master" && secretsLocked() {
		return errors.New("Unlock the passwords with the master password first.")
	}
	if cur.Secrets == "key" {
		if _, err := dataKey(cur); err != nil {
			return errors.New("The current key is not readable: " + err.Error())
		}
	}
	items, err := collectSecrets(cfg)
	if err != nil {
		return errors.New("A secret could not be read: " + err.Error())
	}
	next := cur
	next.Secrets = mode
	var dek []byte
	switch mode {
	case "key":
		if ok, why := encryptedStorage(next.DataDir); !ok {
			return errors.New("The data folder is not on encrypted storage (" + why + "), so a key kept there would protect nothing.")
		}
		dek = make([]byte, 32)
		rand.Read(dek)
		dir := filepath.Dir(keyFile(next))
		os.MkdirAll(dir, 0o700)
		os.Chmod(dir, 0o700)
		if !fileWrite(keyFile(next), []byte(base64.StdEncoding.EncodeToString(dek)+"\n"), 0o600) {
			return errors.New("The key could not be written to " + keyFile(next) + ".")
		}
	case "master":
		if len(password) < 8 {
			return errors.New("The master password needs at least 8 characters.")
		}
		dek = make([]byte, 32)
		rand.Read(dek)
		if !masterStore(password, dek) {
			return errors.New("The master password could not be stored.")
		}
		os.MkdirAll(runSecretsDir(), 0o700)
		fileWrite(unlockedKeyFile(), []byte(base64.StdEncoding.EncodeToString(dek)+"\n"), 0o600)
	}
	for _, it := range items {
		if mode == "plain" {
			if it.kind == "ssh" {
				if !fileWrite(sshPlainKey(it.repo), it.plain, 0o600) {
					return errors.New("Writing a key failed.")
				}
				continue
			}
			if !fileWrite(plainSecretFile(it.repo, it.kind), it.plain, 0o600) {
				return errors.New("Writing a password failed.")
			}
			continue
		}
		enc := seal(dek, it.plain)
		if it.kind == "ssh" {
			if !fileWrite(sshEncKey(it.repo), enc, 0o600) {
				return errors.New("Writing a key failed.")
			}
			continue
		}
		if !fileWrite(encSecretFile(it.repo, it.kind), enc, 0o600) {
			return errors.New("Writing a password failed.")
		}
	}
	for _, it := range items {
		if mode == "plain" {
			if it.kind == "ssh" {
				os.Remove(sshEncKey(it.repo))
			} else {
				os.Remove(encSecretFile(it.repo, it.kind))
			}
			continue
		}
		if it.kind == "ssh" {
			shred(sshPlainKey(it.repo))
		} else {
			shred(plainSecretFile(it.repo, it.kind))
		}
	}
	wipeRunSecrets()
	if cur.Secrets == "key" && mode != "key" {
		shred(keyFile(cur))
	}
	if cur.Secrets == "master" && mode != "master" {
		os.Remove(masterFile())
		masterLock()
	}
	return nil
}

func shred(path string) {
	if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
		os.WriteFile(path, make([]byte, st.Size()), 0o600)
	}
	os.Remove(path)
}

func encryptedStorage(path string) (bool, string) {
	if os.Getenv("RB_ASSUME_ENCRYPTED") == "1" {
		return true, ""
	}
	for _, real := range physicalPaths(path) {
		ok, why := encryptedMount(real)
		if !ok {
			return false, why
		}
	}
	if len(physicalPaths(path)) == 0 {
		return false, "it was not found on any disk or pool"
	}
	return true, ""
}

func physicalPaths(path string) []string {
	share, rest, ok := shareKey(path)
	top := strings.Split(strings.Trim(path, "/"), "/")
	if !ok || len(top) < 2 || (top[1] != "user" && top[1] != "user0") {
		return []string{path}
	}
	var out []string
	entries, _ := os.ReadDir(Mnt)
	for _, e := range entries {
		n := e.Name()
		if inList(n, "user", "user0", "disks", "remotes", "addons", "rootshare") || !e.IsDir() {
			continue
		}
		p := Mnt + "/" + n + "/" + share
		if rest != "" {
			p += "/" + rest
		}
		if exists(p) {
			out = append(out, p)
		}
	}
	return out
}

func encryptedMount(path string) (bool, string) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	var best mountEntry
	for _, m := range mountInfo("self") {
		mp := m.Point
		if (mp == "/" || real == mp || strings.HasPrefix(real, strings.TrimRight(mp, "/")+"/")) && len(mp) >= len(best.Point) {
			best = m
		}
	}
	switch {
	case best.FSType == "zfs":
		ds := best.Source
		_, out, _ := rbCmd([]string{envOr("RB_ZFS", "zfs"), "get", "-H", "-o", "value", "encryption", ds}, 20e9)
		if len(out) > 0 && strings.TrimSpace(out[0]) != "off" && strings.TrimSpace(out[0]) != "-" {
			return true, ""
		}
		pool := strings.SplitN(ds, "/", 2)[0]
		_, st, _ := rbCmd([]string{"zpool", "status", "-P", pool}, 20e9)
		devs, crypt := 0, 0
		for _, l := range st {
			f := strings.Fields(l)
			if len(f) > 0 && strings.HasPrefix(f[0], "/dev/") {
				devs++
				if dmCrypt(f[0]) {
					crypt++
				}
			}
		}
		if devs > 0 && devs == crypt {
			return true, ""
		}
		return false, "ZFS " + ds + " without encryption"
	case strings.HasPrefix(best.Source, "/dev/"):
		if dmCrypt(best.Source) {
			return true, ""
		}
		return false, best.Source + ", " + best.FSType
	}
	return false, best.FSType
}

func dmCrypt(dev string) bool {
	real, err := filepath.EvalSymlinks(dev)
	if err != nil {
		return false
	}
	name := filepath.Base(real)
	b, err := os.ReadFile("/sys/class/block/" + name + "/dm/uuid")
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(b), "CRYPT-")
}
