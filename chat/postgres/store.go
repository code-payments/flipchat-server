//go:build notimplemented

package postgres

import (
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type store struct {
	client *db.PrismaClient
}

/*
func NewPostgres(client *db.PrismaClient) account.Store {
	return &store{
		client,
	}
}
*/
