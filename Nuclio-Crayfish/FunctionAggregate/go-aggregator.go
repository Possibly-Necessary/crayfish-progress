package main

import (
	"context" // For OTel instrumenting
	"log"
	"math" // For OTel instrumenting
	// For OTel instrumenting
	"strconv"
	"strings"
	"time" // For OTel instrumenting
	"github.com/go-redis/redis"
	"github.com/rs/xid"
	// For OTel instrumentation + Jaeger
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	//"go.opentelemetry.io/otel/exporters/jaeger" --> This is deprecated
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	service     = "go-gopher-opentelemetry"
	environment = "development"
	id          = 1
)

// A function that initiates a connection to Jaeger tracer provider
func traceProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create Jaeger exporter -------> deprecated
	/*
		exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
		if err != nil {
			return nil, err
		}*/

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	// Using gRPC
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(url), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
			attribute.String("environment", environment),
			attribute.Int64("ID", id),
		)),
	)

	return tp, nil
}

func initRedisClient(ctx context.Context, tr trace.Tracer) (*redis.Client, error) {
	_, span := tr.Start(ctx, "Initializing Redis Client")
	defer span.End()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     "redis-master.default.svc.cluster.local:6379",
		Password: "y2dvso2x4H", // change password when using the other Redis DB 
		DB:       0,
	})

	pong, err := redisClient.Ping().Result()
	if err != nil {
		span.RecordError(err)
		log.Fatalf("Unable to connect to Redis: %v", err)
		return nil, nil
	}

	log.Printf("Redis server ping response: %s", pong)
	log.Println("Connected to Redis server")

	return redisClient, nil
}

func parseStringSliceToFloatSlice(s string) ([]float64, error) {
	// Remove the brackets [] from the string
	s = strings.Trim(s, "[]")

	// Split the string into a slice of strings
	strs := strings.Split(s, " ")

	//strs :=strings.Split(s, ",")

	// Convert each string in the slice to a float64
	var result []float64
	for _, str := range strs {
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, nil
}

func parseOptimizationResults(values map[string]interface{}) (bestFit float64, bestPos []float64, globalCov []float64) {
	// Parse the values map to extract bestFit, bestPos, and globalCov
	// Parse bestFit
	bestFitStr := values["bestFitness"].(string)
	bestFit, _ = strconv.ParseFloat(bestFitStr, 64)

	// Parse bestPos
	bestPosStr := values["bestPosition"].(string)
	bestPos, _ = parseStringSliceToFloatSlice(bestPosStr)

	// Parse globalCov
	globalCovStr := values["globalCov"].(string)
	globalCov, _ = parseStringSliceToFloatSlice(globalCovStr)
	return
}

func updateOverallResults(overallBestFit *float64, overallBestPos *[]float64, overallGlobalCov *[]float64, bestFit float64, bestPos []float64, globalCov []float64) {
	// Update overall best fitness and position
	if bestFit < *overallBestFit {
		*overallBestFit = bestFit
		*overallBestPos = make([]float64, len(bestPos))
		copy(*overallBestPos, bestPos)
	}

	// Accumulate global convergence values
	if *overallGlobalCov == nil {
		*overallGlobalCov = make([]float64, len(globalCov))
	}
	for i, cov := range globalCov {
		(*overallGlobalCov)[i] += cov
	}
}

func ensureStreamExists(client *redis.Client, streamName string) error {
	// Attempt to add a dummy message to ensure the stream exists
	// Use the smallest possible ID to not interfere with future messages
	_, err := client.XAdd(&redis.XAddArgs{
		Stream: streamName,
		ID:     "0-0",
		Values: map[string]interface{}{
			"setup": "initial setup",
		},
	}).Result()

	if err != nil {
		log.Printf("failed to ensure stream exists: %v", err)
		return err
	}
	return nil
}

func main() {

	var (
		overallBestFit   = math.Inf(1)
		overallBestPos   []float64
		overallGlobalCov []float64
	)
	messageCount := 0
	totalWorkers := 4 // Number of sub-populations

	// Tracer
	tp, err := traceProvider("prod-controller.observability.svc.cluster.local:4317") // <svc-pod-name>.<namespace>.svc.cluster.local:<port> ; using port 4317 for OTel protocol over gRPC
	if err != nil {
		log.Fatal("Failed to create a trace provider", err)
	}

	otel.SetTracerProvider(tp)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Flush telemetry once the aggregator function exits
	defer func() {
		ctx, cancel := context.WithTimeout(ctx, time.Second*3)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	tr := tp.Tracer("component-main")
	ctx, span := tr.Start(ctx, "go-aggregator")
	defer span.End()

	log.Println("Setting up Redis client")

	redisClient, err := initRedisClient(ctx, tr)
	if err != nil {
		span.RecordError(err)
		log.Printf("Failed to initialize a new Redis client: %v\n", err)
	}
	log.Println("Connected to Redis server")

	subject := "optimization_results"
	consumersGroup := "optimization-consumer-group"

	if err := ensureStreamExists(redisClient, subject); err != nil {
		log.Fatalf("Error ensuring stream sxists: %v", err)
	}

	err = redisClient.XGroupCreateMkStream(subject, consumersGroup, "$").Err()
	if err != nil {
		if !strings.Contains(err.Error(), "Consumer group name already exists") {
			log.Println("Consumer Group already exists, moving on...")
		} else {
			log.Printf("Failed to create consumer group: %v", err)
		}
	}

	uniqueID := xid.New().String() // Give each consumer a unique consumer ID

	// Span for message processing
	_, aggregateSpan := tr.Start(ctx, "aggregate-results")
	defer aggregateSpan.End()

	// Listen to Redis channel until there're incoming sub-populations
	for {
		for messageCount < totalWorkers {
			entries, err := redisClient.XReadGroup(&redis.XReadGroupArgs{ // Read results using 'XReadGroup'
				Group:    consumersGroup,
				Consumer: uniqueID, // Use uniqueID here
				Streams:  []string{subject, ">"},
				Count:    2,
				Block:    0,
				NoAck:    false,
			}).Result()
			if err != nil {
				log.Fatal(err)
			}

			// Iterate over each in the entry and process them
			for _, message := range entries[0].Messages {
				bestFit, bestPos, globalCov := parseOptimizationResults(message.Values)
				updateOverallResults(&overallBestFit, &overallBestPos, &overallGlobalCov, bestFit, bestPos, globalCov)
				redisClient.XAck(subject, consumersGroup, message.ID) // Acknowledge
				messageCount++

				if messageCount >= totalWorkers {
					break
				}
			}
		}

		// Average the global convergence values
		for i := range overallGlobalCov {
			overallGlobalCov[i] /= float64(totalWorkers)
		}

		log.Println("Overall Best Fitness:", overallBestFit)
		log.Println("Overall Best Position:", overallBestPos)
		log.Println("Overall Global Convergence:", overallGlobalCov)

		// Reset message count for the next batch
		messageCount = 0
	}

}
