package main

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func main() {
	// Подключение к RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@rabbitmq:5672/")
	failOnError(err, "Не удалось подключиться к RabbitMQ")
	defer conn.Close()

	// Открытие канала
	ch, err := conn.Channel()
	failOnError(err, "Не удалось открыть канал")
	defer ch.Close()

	// Подписка на очередь
	msgs, err := ch.Consume(
		"queue.sp5.14", // очередь
		"",                // consumer tag
		true,              // auto-ack
		false,             // exclusive
		false,             // no-local
		false,             // no-wait
		nil,               // arguments
	)
	failOnError(err, "Не удалось подписаться на очередь")

	log.Println(" [*] Ожидание сообщений. Для выхода нажмите CTRL+C")

	// Чтение сообщений
	forever := make(chan bool)
	go func() {
		for d := range msgs {
			log.Printf(" [x] Получено сообщение: %s", d.Body)
		}
	}()

	<-forever
}
