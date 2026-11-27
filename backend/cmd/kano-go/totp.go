package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yexca/kano-navi/backend/internal/adminauth"
	"github.com/yexca/kano-navi/backend/internal/adminauth/totpadmin"
	"github.com/yexca/kano-navi/backend/internal/config"
)

// runTOTPSecret prints a new one-time code secret as an environment line and
// its otpauth:// address as a comment, for a password manager or
// authenticator app to import. It reads no environment and stores nothing.
func runTOTPSecret(args []string, out io.Writer) int {
	if len(args) != 0 {
		return fail("totp-secret takes no arguments", 2)
	}
	secret, err := adminauth.GenerateTOTPSecret(nil)
	if err != nil {
		return fail("secret could not be generated", 1)
	}
	fmt.Fprintf(out, "ADMIN_TOTP_SECRET=%s\n# %s\n", secret, adminauth.TOTPURI(secret))
	return 0
}

// runTOTPReset removes the one-time code secret bound in the console, for an
// operator who lost the authenticator. It opens an existing database only,
// never creates one, and needs no other environment; the running service
// keeps the code until it restarts.
func runTOTPReset(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("kano-go totp-reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", config.DefaultDatabasePath, "path to the SQLite database")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return fail("invalid arguments for totp-reset", 2)
	}
	path, err := filepath.Abs(*database)
	if err != nil {
		return fail("database unavailable", 1)
	}
	if info, statErr := os.Stat(path); statErr != nil || !info.Mode().IsRegular() {
		return fail("database unavailable", 1)
	}
	// mode=rw opens the file only if it exists.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw&_pragma=busy_timeout(5000)")
	if err != nil {
		return fail("database unavailable", 1)
	}
	defer db.Close()
	removed, err := totpadmin.Reset(context.Background(), db)
	if err != nil {
		return fail("database unavailable", 1)
	}
	if !removed {
		fmt.Fprintln(out, "No one-time code is bound in the console.")
		return 0
	}
	fmt.Fprintln(out, "The console's one-time code is removed. Restart the service; login then asks for the password alone unless ADMIN_TOTP_SECRET is set.")
	return 0
}
