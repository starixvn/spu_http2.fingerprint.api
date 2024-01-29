package main

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func redisPush(client *redis.Client, key string, value string) error {
	ctx := context.Background()

	// Use SetNX to set the value only if the key does not exist
	setResult, err := client.SetNX(ctx, key, value, 0).Result()
	if err != nil {
		return err
	}

	// If the key already existed, replace the value
	if !setResult {
		_, err := client.Set(ctx, key, value, 0).Result()
		if err != nil {
			return err
		}
	}

	return nil
}
