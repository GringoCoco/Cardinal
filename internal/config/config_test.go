package config

import "testing"

func TestNotificationDefaultsAndOverrides(t *testing.T) {
	cfg := &Config{}
	if !cfg.NotifyPermissions() || !cfg.NotifyCompleted() || !cfg.NotifyErrors() {
		t.Fatal("legacy config must keep notifications enabled")
	}
	cfg.Notifications = &NotificationSettings{Permissions: true}
	if !cfg.NotifyPermissions() || cfg.NotifyCompleted() || cfg.NotifyErrors() {
		t.Fatal("explicit notification settings were not applied")
	}
}
