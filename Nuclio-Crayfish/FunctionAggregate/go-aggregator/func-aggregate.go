package main

import (
	"encoding/json"
	"log"
	"math"

	//"strconv"
	//"strings"
	"fmt"
	"time"

	"github.com/go-redis/redis"
)

func initRedisClient() (*redis.Client, error) {

	redisClient := redis.NewClient(&redis.Options{
		Addr:     "redis-master.default.svc.cluster.local:6379",
		Password: "NPUC76ahsT", // change password when using the other Redis DB
		DB:       0,
	})

	pong, err := redisClient.Ping().Result()
	if err != nil {
		log.Fatalf("Unable to connect to Redis: %v", err)
		return nil, nil
	}

	log.Printf("Redis server ping response: %s", pong)
	log.Println("Connected to Redis server")

	return redisClient, nil
}

/*
func parseStringSliceToFloatSlice(s string) ([]float64, error) {

	s = strings.Trim(s, "[]")

	strs := strings.Split(s, " ")

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
	// Parse bestPos
	bestPosStr := values["Best-Position"].(string)
	bestPos, _ = parseStringSliceToFloatSlice(bestPosStr)

	// Parse bestFit
	bestFitStr := values["Best-Fitness"].(string)
	bestFit, _ = strconv.ParseFloat(bestFitStr, 64)

	// Parse globalCov
	globalCovStr := values["Global-Convergance:"].(string)
	globalCov, _ = parseStringSliceToFloatSlice(globalCovStr)

	return bestFit, bestPos, globalCov
}*/

func parseOptimizationResults(values map[string]interface{}) (bestFit float64, bestPos []float64, globalCov []float64) {
	var (
		ok  bool
		err error
	)

	if bestPosIntrf, ok := values["Best-Position"].([]interface{}); ok {
		bestPos, err = interfaceSliceToFloatSlice(bestPosIntrf)
		if err != nil {
			return
		}
	} else {
		log.Printf("Best Position is not a slice of interface{}: %v", err)
		return
	}

	if bestFit, ok = values["Best-Fitness"].(float64); !ok {
		log.Println("Best-Fitness is not a float64")
		return
	}

	if globalCovInterf, ok := values["Global-Convergance:"].([]interface{}); ok {
		globalCov, err = interfaceSliceToFloatSlice(globalCovInterf)
		if err != nil {
			return
		}
	} else {
		log.Printf("Global-Convergance is not a slice of interface{}")
		return
	}

	return bestFit, bestPos, globalCov
}

func interfaceSliceToFloatSlice(input []interface{}) ([]float64, error) {
	var result []float64
	for _, v := range input {
		if f, ok := v.(float64); ok {
			result = append(result, f)
		} else {
			return nil, fmt.Errorf("value %v is not a float64", v)
		}
	}
	return result, nil
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

func main() {

	log.Println("Setting up Redis client")

	redisClient, err := initRedisClient()
	if err != nil {
		log.Printf("Failed to initialize a new Redis client: %v\n", err)
	}
	log.Println("Connected to Redis server")

	subject := "optimization_results"
	subscribe := redisClient.Subscribe(subject)
	defer subscribe.Close()

	var kickstart time.Time
	// Listen to Redis channel until there're incoming sub-populations
	for {
		var (
			overallBestFit   = math.Inf(1)
			overallBestPos   []float64
			overallGlobalCov []float64
		)
		messageCount := 0
		totalWorkers := 10 // Number of sub-populations

		for messageCount < totalWorkers {
			kickstart = time.Now()
			msg, err := subscribe.ReceiveMessage()
			if err != nil {
				log.Printf("Error receiving message: %v", err)
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}

			bestFit, bestPos, globalCov := parseOptimizationResults(data)
			updateOverallResults(&overallBestFit, &overallBestPos, &overallGlobalCov, bestFit, bestPos, globalCov)
			messageCount++

			if messageCount >= totalWorkers {
				break
			}

		}

		// Average the global convergence values
		for i := range overallGlobalCov {
			overallGlobalCov[i] /= float64(totalWorkers)
		}

		end := time.Since(kickstart)

		log.Println("Overall Best Fitness:", overallBestFit)
		//log.Println("Overall Best Position:", overallBestPos)
		//log.Println("Overall Global Convergence:", overallGlobalCov)
		log.Printf("Executed in: %v", end)

		messageCount = 0

	}

}
