/*

Command that built this function (initially):

	nuctl deploy nuc-handler \
    	--namespace nuclio \
    	--file function2.yaml \
    	--registry docker.io/arthurmerlin

Command used with the functionconfig.yaml file:

	nuctl deploy nuc-handler \
    	--namespace nuclio \
    	--file functionconfig.yaml \
    	--registry docker.io/arthurmerlin
     
*/

package main

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	benchmarks "nuclio-handler/benchmark"

	"github.com/go-redis/redis"
	"github.com/nuclio/errors"
	"github.com/nuclio/nuclio-sdk-go" // Nuclio
)

// For the RabbitMQ part
type Message struct {
	SubPopulation [][]float64
	Workers       int    // This is k
	T             int    // Iteration
	F             string // Function name
}

// For publishing to Redis streams
func initRedisClient() (*redis.Client, error) {

	redisClient := redis.NewClient(&redis.Options{
		//Addr: fmt.Sprintf("%s:%s", "127.0.0.1", "6379"),
		Addr: "redis-master.default.svc.cluster.local:6379", // Redis connection string within k8s
		Password: "pass", // Password (this is not the actual password)
		DB: 0, // Default DB
	})
	// Ping the Redis server to check the connection
	_, err := redisClient.Ping().Result()
	if err != nil {
		//context.Logger.Error("Unable to connect to Redis: %v\n", err)
		return nil, errors.New("Unable to connect to Redis..\n")
	}

	//context.Logger.InfoWith("Connected to Redis server")
	return redisClient, nil

}

// Function that publishes results to the Redis stream
func publishOptimizationResults(client *redis.Client, channel string, bestFit float64, bestPos, globalCov []float64) error {
	//log.Println("Publishing optimization results to Redis")

	/*   // Below part was for Redis streams
	// Convert bestPos and globalCov to strings for Redis
	//bestPosStr := fmt.Sprintf("%v", bestPos)
	globalCovStr := fmt.Sprintf("%v", globalCov)

	err := client.XAdd(&redis.XAddArgs{ // XAdd method to add data to the Redis stream
		Stream: "optimization_results", // Stream name
		ID:     "",
		Values: map[string]interface{}{
			"bestPosition": bestPosStr,
			"bestFitness":  bestFit,
			"globalCov":    globalCovStr,
		},
	}).Err()
	*/

	message, err := json.Marshal(map[string]interface{}{
		"Best-Position":       bestPos,
		"Best-Fitness":        bestFit,
		"Global-Convergance:": globalCov,
	})

	if err != nil {
		//context.Logger.Error("Failed to publish in Redis..\n")
		errors.New("Failed to publish in Redis..")
	}

	return client.Publish(channel, message).Err()
}

// Function for dynamic benchmark selection
func selectedBenchmark(F string) benchmarks.FunctionType {

	// Create an empty map and map the benchmark function to the strings
	functionMap := map[string]benchmarks.FunctionType{
		"F1":  benchmarks.F1,
		"F2":  benchmarks.F2,
		"F3":  benchmarks.F3,
		"F4":  benchmarks.F4,
		"F5":  benchmarks.F5,
		"F6":  benchmarks.F6,
		"F7":  benchmarks.F7,
		"F8":  benchmarks.F8,
		"F9":  benchmarks.F9,
		"F10": benchmarks.F10,
		"F11": benchmarks.F11,
		"F16": benchmarks.F16,
		"F17": benchmarks.F17,
		"F18": benchmarks.F18,
	}

	// Dynamically select a benchmark funciton
	candidateFunc, ok := functionMap[F]
	if !ok {
		//context.Logger.Error("Function does not exist..\n")
		errors.New("Function does not exist..\n")
	}

	return candidateFunc
}

// _____________ Main Crayfish Algorithm________________
// Equation 4: Mathimatical model of crayfish intake
func p_obj(x float64) float64 {
	return 0.2 * (1 / (math.Sqrt(2*math.Pi) * 3)) * math.Exp(-math.Pow(x-25, 2)/(2*math.Pow(3, 2)))
}

func crayfish(T int, lb, ub []float64, f string, X [][]float64, F benchmarks.FunctionType) (x float64, y, z []float64) { // return bestFit, bestPos

	N := len(X)      // size of the sub-population
	dim := len(X[0]) // dimension of the sub-populationl

	var (
		globalCov   []float64 = make([]float64, T) // zero row vector of size T
		BestFitness           = math.Inf(1)
		BestPos     []float64 = make([]float64, dim)
		fitnessF    []float64 = make([]float64, N)
		GlobalPos   []float64 = make([]float64, dim)
	)

	for i := 0; i < N; i++ {
		fitnessF[i] = F(X[i]) // Get the fitness value from the benchmark function
		if fitnessF[i] < BestFitness {
			BestFitness = fitnessF[i]
			copy(BestPos, X[i])
		}
	}

	// Update best position to Global position
	copy(GlobalPos, BestPos)
	GlobalFitness := BestFitness

	Xf := make([]float64, dim) // For Xshade -- array for the cave
	Xfood := make([]float64, dim)

	Xnew := make([][]float64, N) // Initializing a 2d array
	for i := 0; i < N; i++ {
		Xnew[i] = make([]float64, dim)
	}

	t := 0
	for t < T {
		//Decreasing curve --> Equation 7
		C := 2 - (float64(t) / float64(T))
		//Define the temprature from Equation 3
		tmp := rand.Float64()*15 + 20

		for i := 0; i < dim; i++ { // Calculating the Cave -> Xshade = XL + XG/2
			Xf[i] = (BestPos[i] + GlobalPos[i]) / 2
		}
		copy(Xfood, BestPos) // copy the best position to the Xfood vector

		for i := 0; i < N; i++ {
			//Xnew[i] = make([]float64, dim) //--> took this part out
			if tmp > 30 { // Summer resort stage
				if rand.Float64() < 0.5 {
					for j := 0; j < dim; j++ { // Equation 6
						Xnew[i][j] = X[i][j] + C*rand.Float64()*(Xf[j]-X[i][j])
					}
				} else { // Competition Stage
					for j := 0; j < dim; j++ {
						z := rand.Intn(N) // Random crayfish
						//z := math.Round(rand.Float64()*(N-1)) + 1 //--> or try this
						Xnew[i][j] = X[i][j] - X[z][j] + Xf[j] // Equation 8
					}
				}
			} else { // Foraging stage
				P := 3 * rand.Float64() * fitnessF[i] / F(Xfood)
				if P > 2 {
					//Food is broken down becuase it's too big
					for j := 0; j < dim; j++ {
						Xfood[j] *= math.Exp(-1 / P)
						Xnew[i][j] = X[i][j] + math.Cos(2*math.Pi*rand.Float64())*Xfood[j]*p_obj(tmp) - math.Sin(2*math.Pi*rand.Float64())*Xfood[j]*p_obj(tmp)
					} // ^^ Equation 13: crayfish foraging
				} else {
					for j := 0; j < dim; j++ { // The case where the food is a moderate size
						Xnew[i][j] = (X[i][j]-Xfood[j])*p_obj(tmp) + p_obj(tmp)*rand.Float64()*X[i][j]
					}
				}
			}
		}

		// Boundary conditions checks
		for i := 0; i < N; i++ {
			for j := 0; j < dim; j++ {
				if len(ub) == 1 {
					Xnew[i][j] = math.Min(ub[0], Xnew[i][j])
					Xnew[i][j] = math.Max(lb[0], Xnew[i][j])
				} else {
					Xnew[i][j] = math.Min(ub[j], Xnew[i][j])
					Xnew[i][j] = math.Max(lb[j], Xnew[i][j])
				}
			}
		}

		//Global update stuff
		copy(GlobalPos, Xnew[0])
		GlobalFitness = F(GlobalPos)

		for i := 0; i < N; i++ {
			NewFitness := F(Xnew[i])
			if NewFitness < GlobalFitness {
				GlobalFitness = NewFitness
				copy(GlobalPos, Xnew[i])
			}

			// Update population to a new location
			if NewFitness < fitnessF[i] {
				fitnessF[i] = NewFitness
				copy(X[i], Xnew[i])
				if fitnessF[i] < BestFitness {
					BestFitness = fitnessF[i]
					copy(BestPos, X[i])
				}
			}
		}

		globalCov[t] = GlobalFitness

		t++
	}

	return BestFitness, BestPos, globalCov
}

// ________________________________________________________________
func NuclioHandler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {

	// Check if the event is triggered by RabbitMQ
	if event.GetTriggerInfo().GetClass() != "async" || event.GetTriggerInfo().GetKind() != "rabbitMq" {
		// If not triggered by RabbitMQ, do nothing and return successfully
		return nil, nil
	}

	// Initialize variable to get the data from the event (RabbitMQ)
	var rabbitData Message

	// For logging/troubleshooting
	context.Logger.InfoWith("Raw message body", "data", string(event.GetBody()))

	err := json.Unmarshal(event.GetBody(), &rabbitData)

	if err != nil {
		log.Printf("Failed to decode message: %v", err)
		return nil, err
	}

	// For logging purposes
	context.Logger.InfoWith("RabbitMQ content extracted from the event",
		"SubPop", rabbitData.SubPopulation, // Sub-populations
		"Workers", rabbitData.Workers, // Number of sub-populations
		"T", rabbitData.T, // Iteration number
		"F", rabbitData.F) // Benchmark function

	// Get the benchmar function specifications from rabbitData.F
	specs := benchmarks.GetFunction(rabbitData.F)
	lb := specs.LB
	ub := specs.UB
	//dim := specs.Dim
	F := selectedBenchmark(rabbitData.F) // Get the actual function from the string

	// (Publisher) intialize Redis client
	redisClient, err := initRedisClient() // Establish a new Redis client
	if err != nil {
		context.Logger.Error("Failed to initialize a new Redis Client: %v\n", err)
		return nil, err
	}

	// Redis channel name
	channel := "Optimization_Results"

	// Start crayfish algorithm
	bestFit, bestPos, globalCov := crayfish(rabbitData.T, lb, ub, rabbitData.F, rabbitData.SubPopulation, F)

	// Publish in Redis
	if err := publishOptimizationResults(redisClient, channel, bestFit, bestPos, globalCov); err != nil {
		context.Logger.Error("Failed to publish data to Redis: %v\n", err)
	}

	/*
		return nuclio.Response{ //Nuclio's return signature
			StatusCode:  200,
			ContentType: "application/json",
			Body:        []byte("Results added to Redis Channel\n"),
		}, nil

	*/

	return nil, nil

}
