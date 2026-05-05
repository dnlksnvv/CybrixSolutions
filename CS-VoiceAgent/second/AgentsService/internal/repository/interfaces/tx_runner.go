package interfaces

import "context"

// TxRunner — минимальная абстракция для Mongo transactions.
// Она нужна usecase-слою, чтобы атомарно обновлять несколько коллекций,
// не завязываясь напрямую на MongoDB драйвер.
type TxRunner interface {
	WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

