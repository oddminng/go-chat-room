package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

func main() {
	listener, err := net.Listen("tcp", ":2020")
	if err != nil {
		panic(err)
	}

	go broadcaster()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Panicln(err)
			continue
		}

		go handleConn(conn)
	}
}

type User struct {
	ID             int
	Addr           string
	EnterAt        time.Time
	MessageChannel chan string
}

func (u *User) String() string {
	return u.Addr + ", UID:" + strconv.Itoa(u.ID) + ", Enter At:" +
		u.EnterAt.Format("2006-01-02 15:04:05+8000")
}

type Message struct {
	OwnerID int
	Content string
}

func NewMessage(ownerID int, content string) *Message {
	return &Message{
		OwnerID: ownerID,
		Content: content,
	}
}

var (
	// 新用户到来，通过该 channel 进行登记
	enteringChannel = make(chan *User)
	// 用户离开，通过该 channel 进行登记
	leavingChannel = make(chan *User)
	// 广播专用的用户普通消息 channel，缓冲是尽可能避免出现异常情况堵塞
	messageChannel = make(chan Message, 8)
)

func handleConn(conn net.Conn) {
	defer conn.Close()

	user := &User{
		ID:             GenUserID(),
		Addr:           conn.RemoteAddr().String(),
		EnterAt:        time.Now(),
		MessageChannel: make(chan string, 8),
	}

	go sendMessage(conn, user.MessageChannel)

	msg := Message{
		OwnerID: user.ID,
		Content: "user:`" + strconv.Itoa(user.ID) + "` has enter",
	}

	user.MessageChannel <- "Welcome, " + user.String()
	messageChannel <- msg

	enteringChannel <- user

	var userActive = make(chan struct{})
	go func() {
		d := 5 * time.Minute
		timer := time.NewTimer(d)
		for {
			select {
			case <-timer.C:
				conn.Close()
			case <-userActive:
				timer.Reset(d)
			}
		}
	}()

	input := bufio.NewScanner(conn)
	for input.Scan() {
		msg.Content = strconv.Itoa(user.ID) + ":" + input.Text()
		messageChannel <- msg
		userActive <- struct{}{}
	}

	if err := input.Err(); err != nil {
		log.Println("读取错误: ", err)
	}

	leavingChannel <- user
	msg.Content = "user:`" + strconv.Itoa(user.ID) + "` has left"
	messageChannel <- msg
}

func sendMessage(conn net.Conn, ch <-chan string) {
	for msg := range ch {
		fmt.Fprintf(conn, msg)
	}
}

func broadcaster() {
	users := make(map[*User]struct{})

	for {
		select {
		case user := <-enteringChannel:
			users[user] = struct{}{}
		case user := <-leavingChannel:
			delete(users, user)
			close(user.MessageChannel)
		case msg := <-messageChannel:
			for user := range users {
				if user.ID == msg.OwnerID {
					continue
				}
				user.MessageChannel <- msg.Content
			}
		}
	}
}

// 生成用户 ID
var (
	globalID int
	idLocker sync.Mutex
)

func GenUserID() int {
	idLocker.Lock()
	defer idLocker.Unlock()

	globalID++
	return globalID
}
