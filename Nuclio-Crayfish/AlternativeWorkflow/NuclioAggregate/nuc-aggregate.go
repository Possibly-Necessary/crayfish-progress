package main

import (
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/nuclio/nuclio-sdk-go"
)

const totalWorkers = 5

type Result struct {
	StartTime time.Time
	BestFit   float64
	BestPos   []float64
	GlobalCov []float64
}

type Aggregator struct {
	overallBestFit   float64
	overallBestPos   []float64
	overallGlobalCov []float64
	messageCount     int
	startTime        time.Time
	//endTime time.Time
	mu sync.Mutex
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		overallBestFit: math.Inf(1),
	}
}

func (a *Aggregator) updateOverallResults(result Result) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.messageCount == 0 {
		//a.startTime = time.Now()
		a.startTime = result.StartTime
	}

	if result.BestFit < a.overallBestFit {
		a.overallBestFit = result.BestFit
		a.overallBestPos = make([]float64, len(result.BestPos))
		copy(a.overallBestPos, result.BestPos)
	}

	if a.overallGlobalCov == nil {
		a.overallGlobalCov = make([]float64, len(result.GlobalCov))
	} else {
		for i, cov := range result.GlobalCov {
			a.overallGlobalCov[i] += cov
		}
	}

	a.messageCount++
}

func AggregateHandler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {
	aggregator, ok := context.UserData.(*Aggregator)
	if !ok {
		aggregator = NewAggregator()
		context.UserData = aggregator
	}

	var result Result
	if err := json.Unmarshal(event.GetBody(), &result); err != nil {
		return nil, err
	}

	aggregator.updateOverallResults(result)

	if aggregator.messageCount >= totalWorkers {
		//aggregator.endTime = time.Now()
		endTime := time.Now()
		workflowExecTime := endTime.Sub(aggregator.startTime)
		// Average the global convergence values
		for i := range aggregator.overallGlobalCov {
			aggregator.overallGlobalCov[i] /= float64(totalWorkers)
		}

		context.Logger.InfoWith("Overall Best Fitness", "fitness", aggregator.overallBestFit)
		context.Logger.InfoWith("Total duration", "duration", workflowExecTime)

		aggregator.messageCount = 0
		aggregator.overallBestFit = math.Inf(1)
		aggregator.overallBestPos = nil
		aggregator.overallGlobalCov = nil
		aggregator.startTime = time.Time{}
		//aggregator.endTime = time.Time{}
	}

	return nil, nil
}
