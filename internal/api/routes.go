package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func logResponse(endpoint string, client string, code uint) {
	currentTime := time.Now().Format("15:04:05")
	fmt.Printf("%s - %s (%d) %s\n", currentTime, endpoint, code, client)
}

// Need body to include the info hash or if none then return all in json
// Headers: Auth, time
func GetInfo(writer http.ResponseWriter, reader *http.Request) {
	host := reader.RemoteAddr
	if reader.Method != http.MethodPost {
		http.Error(writer, "Invalid request method", http.StatusMethodNotAllowed)
		logResponse("getInfo", host, http.StatusMethodNotAllowed)
		return
	}
	err := Authenticate(DevAlwaysTrue)
	if err != nil {
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
		logResponse("getInfo", host, http.StatusUnauthorized)
		return
	}

	bodyBytes, err := io.ReadAll(reader.Body)

	defer reader.Body.Close()

	if err != nil {
		http.Error(writer, "Error reading request body", http.StatusInternalServerError)
		logResponse("getInfo", host, http.StatusInternalServerError)
		return
	}

	var requestBody map[string]interface{}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		http.Error(writer, "Error parsing request body", http.StatusBadRequest)
		logResponse("getInfo", host, http.StatusBadRequest)
		return
	}

	has_hashes := true
	infoHashes, ok := requestBody["info_hashes"].([]interface{})
	if !ok {
		has_hashes = false
	}

	ret_str := ""

	if has_hashes {
		for i, hash := range infoHashes {
			fmt.Printf("Item: %d: %s", i, hash)
			ret_str += fmt.Sprintf("Item: %d: %s", i, hash)
		}
		fmt.Println("has hashes...")
	} else {
		fmt.Println("has no hashes...")
	}
	writer.WriteHeader(http.StatusOK)
	_, err = writer.Write([]byte(ret_str))
	if err != nil {
		fmt.Printf("Error writing response")
	}

}