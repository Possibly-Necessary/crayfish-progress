// yaml file for this is called "function1.yaml"
/*
	nuclt command to build this function:

	nuctl deploy entry-point \
    	--namespace nuclio \
    	--path . \
    	--runtime golang \
    	--handler entry-point:EntryHandler \
    	--registry docker.io/arthurmerlin
*/
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"math/rand"
	benchmark "nuclio-entry/benchmark"

	"github.com/nuclio/errors"
	"github.com/nuclio/nuclio-sdk-go" // Nuclio
	"github.com/streadway/amqp"
)

// For the HTTP trigger; to store the incoming values
type InitParams struct {
	N  int    `json:"n"`  // Population
	K  int    `json:"k"`  // # of sub-population
	T  int    `json:"t"`  // Iteration
	Fn string `json:"fn"` // Function
}

// For the RabbitMQ part
type Message struct {
	SubPopulation [][]float64
	Workers       int    // This is k
	T             int    // Iteration
	F             string // Function name
}

// Funtion to initialize and divide the population
func initializePopulation(N, k, t int, fn string, ch *amqp.Channel, queueName string) error { // Instead of returning ([]byte, error)

	// Get the benchmark function data
	funcData := benchmark.GetFunction(fn) // will hold the string name of the function e.g. "F6"
	lb := funcData.LB
	ub := funcData.UB
	dim := funcData.Dim

	//dim := len(X[0])

	// Initialize the population N x Dim matrix, X
	X := make([][]float64, N)
	for i := 0; i < N; i++ {
		X[i] = make([]float64, dim)
	}

	for i := range X {
		for j := range X[i] {
			X[i][j] = rand.Float64()*(ub[0]-lb[0]) + lb[0]
		}
	}

	// Split the population based on k
	totalSize := len(X)
	baseSubPopSize := totalSize / k // N/k
	remainder := totalSize % k

	Xsub := make([][][]float64, k)

	startIndex := 0
	//subPopCount := 0

	for i := 0; i < k; i++ {
		subPopSize := baseSubPopSize
		if remainder > 0 { // In case the division is not even
			subPopSize++ // Add one of the remaining individuals to this sub-population
			remainder--
		}
		Xsub[i] = X[startIndex : startIndex+subPopSize]
		startIndex += subPopSize

		msg := Message{
			SubPopulation: Xsub[i],
			Workers:       k,
			T:             t,
			F:             fn,
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(msg); err != nil {
			errors.New("Failed to encode message..")
		}

		err := ch.Publish(
			"",        // Exchange (default)
			queueName, // Routing key
			false,     // Mandatory
			false,     // Immediate
			amqp.Publishing{
				ContentType: "application/octet-stream",
				Body:        buf.Bytes(),
			})
		if err != nil {
			errors.New("Failed to publish message..")
		}

		//subPopCount++
	}

	return nil

}

// The Nuclio function that will recieve the HTTP Request
// Publisher for RabbitMQ's exchange
func EntryHandler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {

	var params InitParams
	err := json.Unmarshal(event.GetBody(), &params)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal request body: %w", err)
	}
	// From the HTTP request: T, N, K, F
	// Then, from F, get the lb, ub, dim

	//Initialize RabbitMQ connection and channel
	// Connection was "amqp://nuclio:crayfish@rabbitmq:5672/"
	conn, err := amqp.Dial("amqp://nuclio:crayfish@rabbitmq.default.svc.cluster.local:5672/")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		context.Logger.Error("Failed to establish channel connection: %v", err)
	}
	defer ch.Close()

	// Declare a (default) RabbitMQ queue
	q, err := ch.QueueDeclare(
		"subPopQueue", // QueueName
		false,         // Durable
		false,         // Delete when unused
		false,         // Exclusive
		false,         // No-wait
		nil,           // Arguments
	)
	if err != nil {
		context.Logger.Error("Failed to declare queue: %v", err)
	}

	// Initialize and divide the population -- and that function will
	err = initializePopulation(params.N, params.K, params.T, params.Fn, ch, q.Name)
	if err != nil {
		context.Logger.Error("Failed to intialize population: %v", err)
	}

	return nuclio.Response{
		StatusCode:  200,
		ContentType: "application/json",
		Body:        []byte("Population initialization and distribution completed"),
	}, nil
}
