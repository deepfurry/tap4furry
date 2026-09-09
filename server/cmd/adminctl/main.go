// adminctl is a private operator tool, never part of the Admin HTTP surface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/deepfurry/gofurry-platform/server/internal/auth"
	"github.com/deepfurry/gofurry-platform/server/internal/config"
	"github.com/deepfurry/gofurry-platform/server/internal/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: adminctl grant-role|revoke-role|list-roles -email ADDRESS [-role moderator|editor|admin] [-database gfp_dev|gfp_ci]")
	}
	command := args[0]
	if command != "grant-role" && command != "revoke-role" && command != "list-roles" {
		return errors.New("unsupported adminctl operation")
	}
	flags := flag.NewFlagSet("adminctl", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	email := flags.String("email", "", "existing local account email")
	role := flags.String("role", "", "static role")
	expectedDB := flags.String("database", "gfp_dev", "expected database")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return errors.New("invalid adminctl arguments (values withheld)")
	}
	if *expectedDB != "gfp_dev" && *expectedDB != "gfp_ci" {
		return errors.New("adminctl target must be gfp_dev or disposable gfp_ci")
	}
	c, err := config.Load("migrator")
	if err != nil {
		return err
	}
	if *expectedDB == "gfp_dev" && (os.Getenv("CI") != "" || c.Environment != "development") {
		return errors.New("shared role operations require a local development launch")
	}
	if *expectedDB == "gfp_ci" && (os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1") {
		return errors.New("gfp_ci requires disposable CI guards")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err = database.Inspect(ctx, pool, "gfp_migrator", *expectedDB); err != nil {
		return err
	}
	operator := auth.NewRoleOperator(pool)
	var roles []auth.Role
	switch command {
	case "grant-role":
		roles, err = operator.Grant(ctx, *email, auth.Role(*role))
	case "revoke-role":
		roles, err = operator.Revoke(ctx, *email, auth.Role(*role))
	case "list-roles":
		if *role != "" {
			return errors.New("list-roles does not accept a role")
		}
		roles, err = operator.Roles(ctx, *email)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Role operation complete; current roles: %v\n", roles)
	return nil
}
