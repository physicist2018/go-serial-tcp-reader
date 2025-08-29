package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	fmt.Println("Тестовый сервер запущен на :8080")
	fmt.Println("Генерация CSV данных...")

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		go func(c net.Conn) {
			defer c.Close()

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			counter := 0
			for range ticker.C {
				counter++
				data := fmt.Sprintf("%s,temperature,%.1f,C\n",
					time.Now().Format("2006-01-02 15:04:05"),
					20.0+float64(counter%10)*0.5)
				c.Write([]byte(data))
				fmt.Print("Отправлено: ", data)
			}
		}(conn)
	}
}
