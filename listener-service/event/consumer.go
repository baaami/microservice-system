package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn      *amqp.Connection
	queueName string
}

func NewConsumer(conn *amqp.Connection) (Consumer, error) {
	consumer := Consumer{
		conn: conn,
	}

	err := consumer.setup()
	if err != nil {
		return Consumer{}, err
	}

	return consumer, nil
}

func (consumer *Consumer) setup() error {
	channel, err := consumer.conn.Channel()
	if err != nil {
		return err
	}

	return declareExchange(channel)
}

type Payload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func (consumer *Consumer) Listen(topics []string) error {
	// rabbitMQ 서버와 통신할 채널 'ch'를 생성
	// 해당 채널은 메시지 송/수신 시 사용
	ch, err := consumer.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// 이 큐는 RabbitMQ 서버에서 랜덤하게 이름이 지정되며, 메시지가 저장되는 장소
	q, err := declareRandomQueue(ch)
	if err != nil {
		return err
	}

	// topics 배열에 있는 각 주제(topic)마다 QueueBind를 호출하여 큐를 특정 교환기(exchange)에 바인딩
	for _, s := range topics {
		ch.QueueBind(
			q.Name,
			s,
			"logs_topic",
			false,
			nil,
		)
	}

	// 메시지 소비 시작

	// Consume 함수를 호출하여 큐에서 메시지를 수신
	// 이 함수는 메시지를 비동기적으로 수신할 수 있는 Go 채널(messages)을 반환
	messages, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return err
	}

	// 메시지를 수신할 채널 messages에서 메시지를 비동기적으로 처리하는 고루틴(Goroutine)을 시작
	forever := make(chan bool)
	go func() {
		for d := range messages {
			var payload Payload
			_ = json.Unmarshal(d.Body, &payload)

			go handlePayload(payload)
		}
	}()

	//  블로킹 상태로 유지되어 프로그램이 메시지를 지속적으로 수신하고 처리할 수 있게 함
	fmt.Printf("Waiting for message [Exchange, Queue] [logs_topic, %s]\n", q.Name)
	<-forever

	return nil
}

func handlePayload(payload Payload) {
	switch payload.Name {
	case "log", "event":
		// log whatever we get
		err := logEvent(payload)
		if err != nil {
			log.Println(err)
		}

	case "auth":
		// authenticate
		// you can have as many cases as you want, as long as you write the logic

	default:
		err := logEvent(payload)
		if err != nil {
			log.Println(err)
		}
	}
}

func logEvent(entry Payload) error {
	jsonData, _ := json.MarshalIndent(entry, "", "\t")

	logServiceURL := "http://logger-service/log"

	request, err := http.NewRequest("POST", logServiceURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return err
	}

	return nil
}
