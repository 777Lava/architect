package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const (
	group  = "Б05-123"
	number = "07"
	key    = "student:" + group + ":" + number
)

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Строка
	rdb.Set(ctx, key, "Иванов Иван Иванович", 0)

	// Hash
	rdb.HSet(ctx, key+":info", map[string]interface{}{
		"name":  "Иванов Иван Иванович",
		"age":   "20",
		"email": "ivanovii@misis.edu",
	})

	// List
	rdb.RPush(ctx, key+":timetable", "Математика", "Физика", "Информатика")

	// Set
	rdb.SAdd(ctx, key+":skills", "Golang", "Docker", "PostgreSQL")

	// ZSet
	rdb.ZAdd(ctx, key+":tasks_w_priority", redis.Z{Score: 100, Member: "Сделать лабу 1"})
	rdb.ZAdd(ctx, key+":tasks_w_priority", redis.Z{Score: 150, Member: "Сделать лабу 2"})

	// TTL
	rdb.Expire(ctx, key+":skills", 1*time.Hour)

	// Cache-aside example
	val, err := rdb.Get(ctx, "user:123").Result()
	if err == redis.Nil {
		fmt.Println("Нет в кэше — читаем из БД (условно)")
		dataFromDB := "TestUserFromDB"
		rdb.Set(ctx, "user:123", dataFromDB, time.Minute)
		val = dataFromDB
	}
	fmt.Println("User from cache/DB:", val)

	// Удаление ключа после изменения
	rdb.Del(ctx, "user:123")
}