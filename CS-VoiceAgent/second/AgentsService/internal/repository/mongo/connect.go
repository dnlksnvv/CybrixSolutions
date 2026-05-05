package mongo

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/agents-service/internal/config"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Connect создаёт MongoDB client, пингует базу и возвращает готовое подключение.
// Мы отделяем подключение от бизнес-логики: usecase и handlers никогда не должны знать о деталях драйвера.
func Connect(ctx context.Context, cfg config.MongoConfig) (*driver.Client, error) {
	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	client, err := driver.Connect(connectCtx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancelPing()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	return client, nil
}

