// Subscriber Dapr (Nuclio) function
package main

import ( // Need to add the logic part of extracting the function from the stream
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dapr/go-sdk/service/common"
	daprd "github.com/dapr/go-sdk/service/http"
	"github.com/go-redis/redis"
)

// Using Dapr's SDK

var sub = &common.Subscription{
	PubsubName: "optimization-results-subscription",
	Topic:      "optimization_results",
	Route:      "/process",
}

func parseStringSliceToFloatSlice(s string) ([]float64, error) {
	// Remove the brackets [] from the string
	s = strings.Trim(s, "[]")

	// Split the string into a slice of strings
	strs := strings.Split(s, " ")

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

func eventHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	log.Printf("Subscriber received: %s", e.Data)
	return false, redis.Nil
}

func main() { //Make this a Nuclio funciton and

	// ______________ Dapr SDK part_________________________
	s := daprd.NewService(":6002")
	// Subscribe to a topic
	if err := s.AddTopicEventHandler(sub, eventHandler); err != nil {
		log.Fatalf("error adding topic subscription: %v", err)
	}

	if err := s.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("error listening: %v, err")
	}
	/*
		//_______________________________ Redis Stuff  Below__________________________
		log.Println("Consumer Started")

		redisClient := redis.NewClient(&redis.Options{ // Initialize Redis client
			Addr: fmt.Sprintf("%s:%s", "127.0.0.1", "6379"),
		})
		_, err := redisClient.Ping().Result() // Ping Redis server
		if err != nil {
			log.Fatal("Unbale to connect to Redis", err)
		}

		log.Println("Connected to Redis server")

		subject := "optimization_results"
		consumersGroup := "optimization-consumer-group"

		// Create consumer group to read from Redis' stream
		err = redisClient.XGroupCreate(subject, consumersGroup, "0").Err()
		if err != nil {
			log.Println(err)
		}

		uniqueID := xid.New().String() // Give each consumer a unique consumer ID

		var (
			overallBestFit   = math.Inf(1)
			overallBestPos   []float64
			overallGlobalCov []float64
		)
		messageCount := 0
		totalWorkers := 4 // Number of sub-populations (I need to pass this through HTTP Dapr)

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
			}
		}

	*/

	// Average the global convergence values
	for i := range overallGlobalCov {
		overallGlobalCov[i] /= float64(totalWorkers)
	}

	fmt.Println("Overall Best Fitness:", overallBestFit)
	fmt.Println("Overall Best Position:", overallBestPos)
	fmt.Println("Overall Global Convergence:", overallGlobalCov)

}
