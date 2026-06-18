package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"rustdesk-server/api/model"
	"time"
)

type ServerCmdService struct{}

// List
func (is *ServerCmdService) List(page, pageSize uint) (res *model.ServerCmdList) {
	res = &model.ServerCmdList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.ServerCmd{})
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.ServerCmds)
	return
}

// Info
func (is *ServerCmdService) Info(id uint) *model.ServerCmd {
	u := &model.ServerCmd{}
	DB.Where("id = ?", id).First(u)
	return u
}

// Delete
func (is *ServerCmdService) Delete(u *model.ServerCmd) error {
	return DB.Delete(u).Error
}

// Create
func (is *ServerCmdService) Create(u *model.ServerCmd) error {
	res := DB.Create(u).Error
	return res
}

// SendCmd 
func (is *ServerCmdService) SendCmd(port int, cmd string, arg string) (string, error) {

	cmd = cmd + " " + arg
	res, err := is.SendSocketCmd("v6", port, cmd)
	if err == nil {
		return res, nil
	}
	//v6пјЊv4
	res, err = is.SendSocketCmd("v4", port, cmd)
	if err == nil {
		return res, nil
	}
	return "", err
}

// SendSocketCmd
func (is *ServerCmdService) SendSocketCmd(ty string, port int, cmd string) (string, error) {
	addr := "[::1]"
	tcp := "tcp6"
	if ty == "v4" {
		tcp = "tcp"
		addr = "127.0.0.1"
	}
	conn, err := net.Dial(tcp, fmt.Sprintf("%s:%v", addr, port))
	if err != nil {
		Logger.Debugf("%s connect to id server failed: %v", ty, err)
		return "", err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd))
	if err != nil {
		Logger.Debugf("%s send cmd failed: %v", ty, err)
		return "", err
	}
	// Read until the server closes the connection or our deadline fires. A single
	// conn.Read() with a fixed-size buffer truncated long responses (the relay's
	// blocklist / blocklist_add reply scales with the number of entries and
	// trivially exceeds 4 KB on a populated server) and returned partial data when
	// the kernel handed us only the first packet of a multi-packet response.
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		Logger.Debugf("%s set read deadline failed: %v", ty, err)
		return "", err
	}
	var resp bytes.Buffer
	// Cap the response so a misbehaving server cannot exhaust memory.
	const maxResponseSize = 1 << 20 // 1 MiB
	_, err = io.Copy(&resp, io.LimitReader(conn, maxResponseSize))
	if err != nil && !errors.Is(err, io.EOF) {
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			if resp.Len() == 0 {
				return "", fmt.Errorf("%s: server did not respond within deadline", ty)
			}
			// partial data arrived before deadline — return what we have
		} else {
			Logger.Debugf("%s read response failed: %v", ty, err)
			return "", err
		}
	}
	return resp.String(), nil
}

func (is *ServerCmdService) Update(f *model.ServerCmd) error {
	return DB.Model(f).Updates(f).Error
}
