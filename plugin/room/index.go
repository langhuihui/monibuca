package plugin_room

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
	"m7s.live/v5"
	. "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

// User 结构体代表一个房间用户
type User struct {
	task.Task
	Subscriber *Subscriber
	ID         string
	StreamPath string
	Token      string `json:"-" yaml:"-"`
	Room       *Room  `json:"-" yaml:"-"`
	net.Conn   `json:"-" yaml:"-"`
	writeLock  sync.Mutex
}

func (u *User) GetKey() string {
	return u.ID
}

func (u *User) Start() (err error) {
	u.Subscriber, err = u.Room.Plugin.Subscribe(u, u.StreamPath)
	if err != nil {
		return err
	}
	u.Subscriber.DataChannel = make(chan pkg.IDataFrame, 10)
	return nil
}

func (u *User) Go() error {
	for data := range u.Subscriber.DataChannel {
		u.writeLock.Lock()
		wsutil.WriteServerText(u.Conn, data.([]byte))
		u.writeLock.Unlock()
	}
	return nil
}

func (u *User) Dispose() {
	close(u.Subscriber.DataChannel)
}

func (u *User) Send(event string, data any) {
	if u.Conn != nil {
		u.writeLock.Lock()
		defer u.writeLock.Unlock()
		j, err := json.Marshal(map[string]any{"event": event, "data": data})
		if err == nil {
			wsutil.WriteServerText(u.Conn, j)
		}
	}
}

type Room struct {
	*Publisher
	ID    string
	Users task.WorkCollection[string, *User]
}

//go:embed default.yaml
var defaultYaml DefaultYaml

type RoomPlugin struct {
	Plugin
	AppName string            `default:"room" desc:"用于订阅房间消息的应用名（streamPath第一段）"`
	Size    int               `default:"20" desc:"房间大小"`
	Private map[string]string `desc:"私密房间" key:"房间号" value:"密码"`
	Verify  struct {
		URL    string            `desc:"验证用户身份的URL"`
		Method string            `desc:"验证用户身份的HTTP方法"`
		Header map[string]string `desc:"验证用户身份的HTTP头" key:"名称" value:"值"`
	} `desc:"验证用户身份"`
	Ping  string `default:"ping" desc:"用于客户端与服务器保持心跳时客户端发送的特殊字符串"`
	Pong  string `default:"pong" desc:"用于客户端与服务器保持心跳时服务器响应的特殊字符串"`
	lock  sync.RWMutex
	rooms util.Collection[string, *Room]
}

var _ = InstallPlugin[RoomPlugin](m7s.PluginMeta{
	DefaultYaml: defaultYaml,
})

func (rc *RoomPlugin) OnPublish(p *Publisher) {
	args := p.Args
	token := args.Get("token")
	ss := strings.Split(token, ":")
	if len(ss) != 3 {
		return
	}
	roomId := ss[0]
	userId := ss[1]
	if roomId != "" && rc.rooms.Has(roomId) {
		room, _ := rc.rooms.Get(roomId)
		if user, ok := room.Users.Get(userId); ok {
			if user.Token == token {
				user.StreamPath = p.StreamPath
				data, _ := json.Marshal(map[string]any{
					"event":  "publish",
					"data":   p.StreamPath,
					"userId": user.ID,
				})
				room.WriteData(data)
			}
		}
	}
}

func (rc *RoomPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ss := strings.Split(r.URL.Path, "/")[1:]
	var roomId, userId, token string
	if len(ss) == 2 {
		roomId = ss[0]
		userId = ss[1]
	} else {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	if rc.Verify.URL != "" {
		req, _ := http.NewRequest(rc.Verify.Method, rc.Verify.URL, nil)
		req.Header = r.Header
		res, _ := http.DefaultClient.Do(req)
		if res.StatusCode != 200 {
			http.Error(w, "verify failed", http.StatusForbidden)
		}
	}
	if rc.Private != nil {
		rc.lock.RLock()
		pass, ok := rc.Private[roomId]
		rc.lock.Unlock()
		if ok {
			if pass != r.URL.Query().Get("password") {
				http.Error(w, "password wrong", http.StatusForbidden)
				return
			}
		}
	}
	var room *Room
	var ok bool
	if room, ok = rc.rooms.Get(roomId); !ok {
		room = &Room{ID: roomId}
		if _, err := rc.Publish(context.Background(), rc.AppName+"/"+roomId); err == nil {
			rc.rooms.Add(room)
		} else {
			http.Error(w, "room already exist", http.StatusBadRequest)
			return
		}
	}
	if room.Users.Has(userId) {
		http.Error(w, "user exist", http.StatusBadRequest)
		return
	}
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()
	token = fmt.Sprintf("%s:%s:%s", roomId, userId, uuid.NewString())
	user := &User{Room: room, Conn: conn, Token: token, ID: userId}
	data, _ := json.Marshal(map[string]any{"event": "userjoin", "data": user})
	room.WriteData(data)
	room.Users.AddTask(user)
	user.Send("joined", map[string]any{"token": token, "userList": room.Users.ToList()})
	defer func() {
		user.Stop(err)
		if room.Users.Length() == 0 {
			room.Stop(err)
			rc.rooms.RemoveByKey(roomId)
		}
	}()
	var msg []byte
	var op ws.OpCode
	for {
		msg, op, err = wsutil.ReadClientData(conn)
		if string(msg) == rc.Ping {
			wsutil.WriteServerText(conn, []byte(rc.Pong))
		} else {
			data, _ := json.Marshal(map[string]any{"event": "msg", "data": string(msg), "userId": userId})
			room.WriteData(data)
		}
		if op == ws.OpClose || err != nil {
			data, _ := json.Marshal(map[string]any{"event": "userleave", "userId": userId, "data": user})
			room.WriteData(data)
			return
		}
	}
}
