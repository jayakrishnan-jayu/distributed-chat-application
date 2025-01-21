package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	conn, err := net.Dial("tcp", "192.168.0.107:5006")
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to server.")

	// Goroutine to handle incoming messages from the server
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("Message from server:", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("Error reading from server:", err)
		}
	}()

	// Main loop to send messages to the server
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter message: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "exit" {
			fmt.Println("Exiting client.")
			break
		}

		_, err = conn.Write([]byte(input + "\n"))
		if err != nil {
			fmt.Println("Error sending message:", err)
			break
		}
	}
}
