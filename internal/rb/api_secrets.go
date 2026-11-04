package rb

func init() {
	registerAction("secrets_status", apiSecretsStatus)
	registerAction("secrets_mode", apiSecretsMode)
	registerAction("secrets_unlock", apiSecretsUnlock)
	registerAction("secrets_lock", apiSecretsLock)
}

func apiSecretsStatus(p M) (M, error) {
	cfg := configLoad()
	s := cfg.Settings
	ok, why := encryptedStorage(s.DataDir)
	return M{
		"mode":           s.Secrets,
		"locked":         s.Secrets == "master" && secretsLocked(),
		"problem":        secretsReady(s),
		"key_file":       keyFile(s),
		"key_possible":   ok,
		"key_impossible": why,
	}, nil
}

func apiSecretsMode(p M) (M, error) {
	mode := pstr(p, "mode", "")
	if !inList(mode, "plain", "key", "master") {
		return nil, apiFail("Choose how the passwords are kept.")
	}
	password := pstr(p, "password", "")
	current := pstr(p, "current_password", "")
	if r := opsRunning(); len(r) > 0 {
		return nil, apiFail("Wait until the running operations have finished.")
	}
	return configChange(func(cfg *Config) (M, error) {
		if cfg.Settings.Secrets == "master" && mode != "master" || cfg.Settings.Secrets == "master" && password != "" {
			if !masterCheck(current) {
				return nil, apiFail("The current master password is wrong.")
			}
		}
		if mode == "master" && password != pstr(p, "password_repeat", password) {
			return nil, apiFail("The two master passwords differ.")
		}
		for _, r := range cfg.Repos {
			browseStop(r.ID, 20)
		}
		if err := secretsSwitch(cfg, mode, password); err != nil {
			return nil, apiFail(err.Error())
		}
		cfg.Settings.Secrets = mode
		return M{"mode": mode}, nil
	})
}

func apiSecretsUnlock(p M) (M, error) {
	if configLoad().Settings.Secrets != "master" {
		return nil, apiFail("No master password is in use.")
	}
	if err := masterUnlock(pstr(p, "password", "")); err != nil {
		return nil, apiFail(err.Error())
	}
	stateUpdate(configLoad().Settings, func(state M) {
		delete(state, "locked_notified")
	})
	return M{"locked": false}, nil
}

func apiSecretsLock(p M) (M, error) {
	if configLoad().Settings.Secrets != "master" {
		return nil, apiFail("No master password is in use.")
	}
	if r := opsRunning(); len(r) > 0 {
		return nil, apiFail("Wait until the running operations have finished.")
	}
	for _, id := range browseRepos() {
		browseStop(id, 20)
	}
	masterLock()
	return M{"locked": true}, nil
}
