package mongo

import (
	"context"
	"fmt"

	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	driver "go.mongodb.org/mongo-driver/mongo"
)

// TxRunner — Mongo реализация repo.TxRunner.
type TxRunner struct {
	client *driver.Client
}

var _ repo.TxRunner = (*TxRunner)(nil)

func NewTxRunner(client *driver.Client) *TxRunner {
	return &TxRunner{client: client}
}

func (t *TxRunner) WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if t.client == nil {
		return fmt.Errorf("mongo tx: client is nil")
	}
	sess, err := t.client.StartSession()
	if err != nil {
		return fmt.Errorf("mongo tx: start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sc driver.SessionContext) (any, error) {
		if err := fn(sc); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

