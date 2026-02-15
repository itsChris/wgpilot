package server

import (
	"context"
	"fmt"
	"time"

	"github.com/itsChris/wgpilot/internal/db"
	servermw "github.com/itsChris/wgpilot/internal/server/middleware"
)

// Compile-time check that apiKeyStoreAdapter implements servermw.APIKeyStore.
var _ servermw.APIKeyStore = (*apiKeyStoreAdapter)(nil)

// apiKeyStoreAdapter adapts *db.DB to the middleware.APIKeyStore interface.
type apiKeyStoreAdapter struct {
	db *db.DB
}

func (a *apiKeyStoreAdapter) GetAPIKeyByHash(ctx context.Context, hash string) (id int64, userID int64, role string, expiresAt *time.Time, err error) {
	k, err := a.db.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return 0, 0, "", nil, fmt.Errorf("get api key by hash: %w", err)
	}
	if k == nil {
		return 0, 0, "", nil, fmt.Errorf("api key not found")
	}
	return k.ID, k.UserID, k.Role, k.ExpiresAt, nil
}

func (a *apiKeyStoreAdapter) UpdateAPIKeyLastUsed(ctx context.Context, id int64) error {
	return a.db.UpdateAPIKeyLastUsed(ctx, id)
}
