/*

Instead of this script, type this command:

curl -X POST http://localhost:3000/test-function \
     -H "Content-Type: application/json" \
     -d '{"N":30, "K":5, "T":100, "F":"F6"}'

*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Struct for COA parameters
type Payload struct {
	N int    `json:"n"` // Number of overall population
	K int    `json:"k"` // Number of sub-populations
	T int    `json:"t"` // Number of iterations for the main COA algorithm
	F string `json:"f"` // Benchmark function
}

func main() {
	// URL of the Nuclio function inside Minikube.
	nuclioFunctionURL := "http://localhost:3000/test-function"
	// "http://<minikube-ip>:<port>/<function-path>"

	// Construct the payload: here we send COA parameters
	payload := Payload{
		N: 30,
		K: 5,
		T: 100,
		F: "F6",
	}

	// Marshal the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshalling payload: %v\n", err)
		return
	}

	// Create a new HTTP POST request with the JSON payload
	req, err := http.NewRequest("POST", nuclioFunctionURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response status: %s\n", resp.Status)

}
