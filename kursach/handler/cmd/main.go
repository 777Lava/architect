package main

import (
	"context"
	"database/sql"
	"fmt"
	"handler/tracer"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

var (
	requestsCounter *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	metrics         *tracer.Metrics
	registry        *prometheus.Registry
)

func setupMetrics() {
	requestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: []float64{0.1, 0.3, 1.0, 2.5, 5.0},
		},
		[]string{"method", "path"},
	)

	metrics = tracer.NewMetrics()

	registry = prometheus.NewRegistry()
	registry.MustRegister(
		requestsCounter,
		requestDuration,
		metrics.BooksCreated,
		metrics.BooksUpdated,
		metrics.BooksDeleted,
		metrics.DBQueryTime,
		metrics.CacheHit,
		metrics.CacheMiss,
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			Namespace: "myapp",
		}),
		collectors.NewGoCollector(),
	)
}

func main() {
	setupMetrics()

	var conn *amqp091.Connection
	var err error

	for i := 0; i < 10; i++ {
		rabbit := os.Getenv("RB_HOST")
		s := fmt.Sprintf("amqp://guest:guest@%s:5672", rabbit)
		fmt.Println("Подключаюсь к RabbitMQ: ", s)
		conn, err = amqp091.Dial(s)
		if err != nil {
			fmt.Println(err, "Failed to connect to RabbitMQ")
		} else {
			fmt.Println("Успешное подключение к RabbitMQ")
			break
		}
		time.Sleep(3 * time.Second)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		fmt.Println("Failed to open a channel")
	}
	defer ch.Close()

	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
	}
	defer db.Close()

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS books (
		id SERIAL PRIMARY KEY,
		description TEXT NOT NULL
	);`

	if _, err = db.Exec(createTableSQL); err != nil {
		fmt.Printf("Ошибка при создании таблицы: %v\n", err)
	}

	rd_host := os.Getenv("RD_HOST")
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:6379", rd_host),
	})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		fmt.Printf("Failed to connect to Redis: %v\n", err)
	} else {
		fmt.Println("подключились к рэдису")
	}

	racer := tracer.Tracer{
		Ch:      ch,
		Db:      db,
		Rdb:     rdb,
		Metrics: metrics,
	}

	router := gin.New()

	router.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		method := c.Request.Method

		c.Next()

		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		requestDuration.WithLabelValues(method, path).Observe(duration)
		requestsCounter.WithLabelValues(method, path, status).Inc()
	})

	books := router.Group("/lib")
	{
		books.POST("", racer.Create)
		books.GET("", racer.GetAll)
		books.PUT("", racer.Update)
		books.DELETE("", racer.Delete)
		books.GET("/:id", racer.GetOne)
	}

	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	fmt.Println("Сервер запущен на :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		fmt.Printf("Ошибка при запуске сервера: %v\n", err)
	}
}
