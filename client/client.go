package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const broadcastPort = 5001
const serverPort = 5006

type Client struct {
	serverAddr string
	conn       net.Conn
	username   string
	term       *term.Terminal
}

func (c *Client) listenForBroadcasts() string {
	fmt.Fprintln(c.term, string(c.term.Escape.Blue)+"Searching: "+string(c.term.Escape.Reset), "leader")
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", broadcastPort))
	conn, _ := net.ListenUDP("udp", addr)

	defer conn.Close()

	buffer := make([]byte, 1)

	prefix := string(c.term.Escape.Blue) + "Broadcast server found: " + string(c.term.Escape.Reset)
	for {
		_, remoteAddr, _ := conn.ReadFromUDP(buffer)
		fmt.Fprintln(c.term, prefix, remoteAddr.IP.String())
		return remoteAddr.IP.String()
	}
}

func (c *Client) connectToServer(ip string) error {
	prefix := string(c.term.Escape.Blue) + "Connected To: " + string(c.term.Escape.Reset)
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, serverPort))
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return err
	}

	fmt.Fprintln(c.term, prefix, ip)
	c.conn = conn
	return nil
}

func (c *Client) sendMessageToServer(input string) error {
	_, err := c.conn.Write([]byte(c.username + ":" + input + "\n"))
	return err
}

func (c *Client) listen() string {
	scanner := bufio.NewScanner(c.conn)
	defaultPrefix := string(c.term.Escape.Cyan) + "Message: " + string(c.term.Escape.Reset)
	for scanner.Scan() {
		text := scanner.Text()
		messages := strings.SplitN(text, ":", 2)
		if len(messages) == 1 {
			fmt.Fprintln(c.term, defaultPrefix, text)
			ip := net.ParseIP(text)
			if ip != nil {
				return text
			}
			continue
		}
		rePrefix := string(c.term.Escape.Cyan) + messages[0] + ":" + string(c.term.Escape.Reset)
		fmt.Fprintln(c.term, rePrefix, messages[1])
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading from server:", err)
	}

	return ""
}

func (c *Client) run() {
	ip := c.listenForBroadcasts()
	c.connectAndListen(ip)
}
func (c *Client) connectAndListen(ip string) {
	c.connectToServer(ip)
	ip = c.listen()
	if ip != "" {
		defaultPrefix := string(c.term.Escape.Blue) + "Leader redirected: " + string(c.term.Escape.Reset)
		fmt.Fprintln(c.term, defaultPrefix, ip)
		go c.connectAndListen(ip)
		return
	}
	defaultPrefix := string(c.term.Escape.Red) + "Connection Closed: " + string(c.term.Escape.Reset)
	fmt.Fprintln(c.term, defaultPrefix, "Trying Again")
	go c.run()
}

func main() {
	var username string
	fmt.Print("Enter Username: ")
	fmt.Scanf("%s\n", &username)
	fmt.Print("\033[2J")
	c := Client{username: username}
	go c.chat()
	time.Sleep(50 * time.Millisecond)
	go c.run()
	for {

	}

	// go c.handleUserInput()
	// c.render()
	// for {
	// }

	// broadcastingServerIp := c.listenForBroadcasts()
	// c.connectToServer(broadcastingServerIp)
	// if c.connected {
	//
	// }

	// conn, err := net.Dial("tcp", "192.168.0.107:5006")
	// if err != nil {
	// 	fmt.Println("Error connecting to server:", err)
	// 	return
	// }
	// defer conn.Close()
	//
	// fmt.Println("Connected to server.")
	//
	// // Goroutine to handle incoming messages from the server
	// go func() {
	// 	scanner := bufio.NewScanner(conn)
	// 	for scanner.Scan() {
	// 		fmt.Println("Message from server:", scanner.Text())
	// 	}
	// 	if err := scanner.Err(); err != nil {
	// 		fmt.Println("Error reading from server:", err)
	// 	}
	// }()
	//
	// // Main loop to send messages to the server
	// reader := bufio.NewReader(os.Stdin)
	// for {
	// 	fmt.Print("Enter message: ")
	// 	input, err := reader.ReadString('\n')
	// 	if err != nil {
	// 		fmt.Println("Error reading input:", err)
	// 		continue
	// 	}
	//
	// 	input = strings.TrimSpace(input)
	// 	if input == "exit" {
	// 		fmt.Println("Exiting client.")
	// 		break
	// 	}
	//
	// 	_, err = conn.Write([]byte(input + "\n"))
	// 	if err != nil {
	// 		fmt.Println("Error sending message:", err)
	// 		break
	// 	}
	// }
}

func (c *Client) chat() error {
	if !term.IsTerminal(0) || !term.IsTerminal(1) {
		return fmt.Errorf("stdin/stdout should be term")
	}
	oldState, err := term.MakeRaw(0)
	if err != nil {
		return err
	}
	defer term.Restore(0, oldState)
	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	term := term.NewTerminal(screen, "")
	term.SetPrompt(string(term.Escape.Red) + "> " + string(term.Escape.Reset))
	c.term = term

	rePrefix := string(term.Escape.Cyan) + "Failed to send message" + string(term.Escape.Reset)
	// go func() {
	// 	for {
	// 		time.Sleep(1 * time.Second)
	// 		fmt.Fprintln(term, rePrefix, "hello")
	// 	}
	// }()

	for {
		line, err := term.ReadLine()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if line == "" {
			continue
		}

		err = c.sendMessageToServer(line)
		if err != nil {
			fmt.Fprintln(term, rePrefix, "Connection closed to server")
			go c.run()
		}
		// fmt.Fprintln(term, rePrefix, line)
	}
}
