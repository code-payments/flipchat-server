package test

import (
	"os"
	"os/exec"
)

// A bit of a hack, we should call the prisma migration programatically
func RunPrismaMigrateDeploy(databaseUrl string) error {
	cmd := exec.Command("go", "run", "github.com/steebchen/prisma-client-go", "migrate", "deploy")
	cmd.Env = append(os.Environ(), "DATABASE_URL="+databaseUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
