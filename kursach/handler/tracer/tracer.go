package tracer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"handler"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type Metrics struct {
	BooksCreated prometheus.Counter
	BooksUpdated prometheus.Counter
	BooksDeleted prometheus.Counter
	DBQueryTime  prometheus.Histogram
	CacheHit     prometheus.Counter
	CacheMiss    prometheus.Counter
}

func NewMetrics() *Metrics {
	return &Metrics{
		BooksCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "books_created_total",
			Help: "Total number of books created",
		}),
		BooksUpdated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "books_updated_total",
			Help: "Total number of books updated",
		}),
		BooksDeleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "books_deleted_total",
			Help: "Total number of books deleted",
		}),
		DBQueryTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "db_query_time_seconds",
			Help:    "Database query execution time",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 2},
		}),
		CacheHit: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		}),
		CacheMiss: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		}),
	}
}

type Tracer struct {
	Ch      *amqp091.Channel
	Db      *sql.DB
	Rdb     *redis.Client
	Metrics *Metrics
}

func (t *Tracer) Create(c *gin.Context) {

	var book handler.Book
	start := time.Now()
	defer func() {
		t.Metrics.DBQueryTime.Observe(time.Since(start).Seconds())
	}()

	if err := c.BindJSON(&book); err != nil {
		fmt.Println(err.Error(), "ne schital")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	jsonData, err := json.Marshal(book)
	if err != nil {
		fmt.Println("Ошибка при сериализации JSON:", err)
		return
	}
	err = t.Ch.PublishWithContext(ctx,
		"do.direct",  // exchange
		"create.key", // routing key
		false,        // mandatory
		false,        // immediate
		amqp091.Publishing{
			ContentType: "text/plain",
			Body:        jsonData,
		})
	if err != nil {
		fmt.Println(err.Error(), "err in send message from POST request")
	}
	fmt.Printf(" [x] Sent %s", book.Description)
	t.Metrics.BooksCreated.Inc()
	c.JSON(http.StatusOK, "created")

}

func (t *Tracer) GetAll(c *gin.Context) {
	rows, err := t.Db.Query("SELECT * FROM books")
	if err != nil {
		fmt.Println("eror in bd injection")
	}
	defer rows.Close()
	books := []handler.Book{}
	for rows.Next() {
		count := handler.Book{}

		err := rows.Scan(&count.Id, &count.Description)
		if err != nil {
			fmt.Printf("Ошибка при сканировании строки: %v\n", err)
		}

		books = append(books, count)
	}
	c.JSON(http.StatusOK, books)

}

func (t *Tracer) GetOne(c *gin.Context) {

	start := time.Now()
	defer func() {
		t.Metrics.DBQueryTime.Observe(time.Since(start).Seconds())
	}()

	var book1 handler.Book
	var err error
	book1.Id, err = strconv.Atoi(c.Param("id"))
	if err != nil {
		fmt.Println("not find id in param")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sid := strconv.Itoa(book1.Id)
	val, err := t.Rdb.Get(ctx, sid).Result()
	if err != nil {
		fmt.Println("error in  rdb.get", err)
	}

	if err == redis.Nil {
		t.Metrics.CacheMiss.Inc()
		fmt.Println("empty in rdb")
		rows, err := t.Db.Query("SELECT id, description FROM books WHERE id = $1", book1.Id)
		if err != nil {
			fmt.Println("eror in bd injection")
		}
		defer rows.Close()
		books := []handler.Book{}
		for rows.Next() {
			count := handler.Book{}

			err := rows.Scan(&count.Id, &count.Description)
			if err != nil {
				fmt.Printf("Ошибка при сканировании строки: %v\n", err)
			}

			books = append(books, count)
		}
		fmt.Println("book", books)
		if len(books) > 0 {
			t.Rdb.Set(ctx, sid, books[0].Description, 0)
			c.JSON(http.StatusOK, books)
		} else {
			c.JSON(http.StatusOK, "there is no one with that ID")
		}
	} else {
		t.Metrics.CacheHit.Inc()
		book := handler.Book{}
		book.Id = book1.Id
		book.Description = val
		fmt.Println("book")
		c.JSON(http.StatusOK, book)

	}

}

func (t *Tracer) Delete(c *gin.Context) {
	var book handler.Book

	if err := c.BindJSON(&book); err != nil {
		fmt.Println(err.Error(), "cannot read ID")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(book)
	if err != nil {
		fmt.Println("Ошибка при сериализации JSON:", err)
		return
	}
	err = t.Ch.PublishWithContext(ctx,
		"do.direct",  // exchange
		"delete.key", // routing key
		false,        // mandatory
		false,        // immediate
		amqp091.Publishing{
			ContentType: "text/plain",
			Body:        jsonData,
		})
	if err != nil {
		fmt.Println(err.Error(), "err in send message from DELETE request")
	}
	fmt.Println(" [x] Sent ", book.Id)

	c.JSON(http.StatusOK, "deleted")
}

func (t *Tracer) Update(c *gin.Context) {
	var book handler.Book

	if err := c.BindJSON(&book); err != nil {
		fmt.Println(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(book)
	if err != nil {
		fmt.Println("Ошибка при сериализации JSON:", err)
		return
	}
	err = t.Ch.PublishWithContext(ctx,
		"do.direct",  // exchange
		"update.key", // routing key
		false,        // mandatory
		false,        // immediate
		amqp091.Publishing{
			ContentType: "text/plain",
			Body:        jsonData,
		})
	if err != nil {
		fmt.Println(err.Error(), "err in send message from PUT request")
	}
	fmt.Println(" [x] Sent ", book)

	c.JSON(http.StatusOK, "updated")
}
