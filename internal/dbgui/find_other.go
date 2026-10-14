//go:build !windows

package dbgui

// FindSQLyog is Windows-only. SQLyog Community is a Windows installer.
func FindSQLyog() string { return "" }
