package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/rabbitmq/amqp091-go"
)

// Book структура для работы с таблицей books
type Book struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

var db *sql.DB

func main() {
	var conn *amqp091.Connection
	var err error

	// Подключение к RabbitMQ
	for i := 0; i < 10; i++ {
		rabbit := os.Getenv("RB_HOST")
		s := fmt.Sprint("amqp://guest:guest@", rabbit, ":5672")
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

	// Подключение к PostgreSQL
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err = sql.Open("postgres", dsn)
	fmt.Println("Подключаюсь к PostgreSQL: ", dsn)
	if err != nil {
		log.Fatalf("Error connecting to database: %v\n", err)
	} else {
		fmt.Println("Успешное подключение к PostgreSQL")
	}
	defer db.Close()

	// Проверка соединения с БД
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	err = ch.ExchangeDeclare(
		"do.direct", // имя exchange
		"direct",    // тип
		true, false, false, false, nil,
	)
	failOnError(err, "Failed to declare exchange")

	// Очередь для создания
	q1, err := ch.QueueDeclare("queue.create", true, false, false, false, nil)
	failOnError(err, "Failed to declare queue")
	err = ch.QueueBind(q1.Name, "create.key", "do.direct", false, nil)
	failOnError(err, "Failed to bind queue")
	fmt.Println("Создали очередь create")

	// Очередь для обновления
	q2, err := ch.QueueDeclare("queue.update", true, false, false, false, nil)
	failOnError(err, "Failed to declare queue")
	err = ch.QueueBind(q2.Name, "update.key", "do.direct", false, nil)
	failOnError(err, "Failed to bind queue")
	fmt.Println("Создали очередь update")

	// Очередь для удаления
	q3, err := ch.QueueDeclare("queue.delete", true, false, false, false, nil)
	failOnError(err, "Failed to declare queue")
	err = ch.QueueBind(q3.Name, "delete.key", "do.direct", false, nil)
	failOnError(err, "Failed to bind queue")
	fmt.Println("Создали очередь delete")

	// Запускаем обработчики для 3 очередей
	go listenQueue(ch, "queue.create", handleCreate)
	go listenQueue(ch, "queue.update", handleUpdate)
	go listenQueue(ch, "queue.delete", handleDelete)

	log.Println(" [*] Слушаем очереди. Нажмите CTRL+C для выхода.")
	select {} // Блокируем основной поток
}

func listenQueue(ch *amqp091.Channel, queueName string, handler func([]byte)) {
	msgs, err := ch.Consume(
		queueName, "", true, false, false, false, nil,
	)
	failOnError(err, "Не удалось подписаться на "+queueName)

	for msg := range msgs {
		log.Printf("[→ %s] Сообщение: %s", queueName, msg.Body)
		handler(msg.Body)
	}
}

func handleCreate(body []byte) {
	var book Book
	err := json.Unmarshal(body, &book)
	if err != nil {
		log.Printf("[CREATE] Ошибка парсинга JSON: %v", err)
		return
	}

	sqlStatement := `
	INSERT INTO books (description)
	VALUES ($1)
	RETURNING id`

	var id int
	err = db.QueryRow(sqlStatement, book.Description).Scan(&id)
	if err != nil {
		log.Printf("[CREATE] Ошибка при создании записи: %v", err)
		return
	}

	log.Printf("[CREATE] Создана новая запись с ID: %d", id)
}

func handleUpdate(body []byte) {
	var book Book
	err := json.Unmarshal(body, &book)
	if err != nil {
		log.Printf("[UPDATE] Ошибка парсинга JSON: %v", err)
		return
	}

	if book.ID == 0 {
		log.Printf("[UPDATE] Для обновления необходимо указать ID книги")
		return
	}

	sqlStatement := `
	UPDATE books 
	SET description = $1 
	WHERE id = $2`

	res, err := db.Exec(sqlStatement, book.Description, book.ID)
	if err != nil {
		log.Printf("[UPDATE] Ошибка при обновлении записи: %v", err)
		return
	}

	count, err := res.RowsAffected()
	if err != nil {
		log.Printf("[UPDATE] Ошибка при проверке обновленных строк: %v", err)
		return
	}

	if count == 0 {
		log.Printf("[UPDATE] Запись с ID %d не найдена", book.ID)
	} else {
		log.Printf("[UPDATE] Успешно обновлена запись с ID %d", book.ID)
	}
}

func handleDelete(body []byte) {
	var book Book
	err := json.Unmarshal(body, &book)

	if err != nil {
		log.Printf("[DELETE] Ошибка парсинга JSON: %v", err)
		return
	}

	if book.ID == 0 {
		log.Printf("[DELETE] Для удаления необходимо указать ID книги")
		return
	}

	sqlStatement := `DELETE FROM books WHERE id = $1`
	res, err := db.Exec(sqlStatement, book.ID)
	if err != nil {
		log.Printf("[DELETE] Ошибка при удалении записи: %v", err)
		return
	}

	count, err := res.RowsAffected()
	if err != nil {
		log.Printf("[DELETE] Ошибка при проверке удаленных строк: %v", err)
		return
	}

	if count == 0 {
		log.Printf("[DELETE] Запись с ID %d не найдена", book.ID)
	} else {
		log.Printf("[DELETE] Успешно удалена запись с ID %d", book.ID)
	}
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}
