/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package protocol

import (
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/transport"
)

type CmdRunner struct {
	conn *transport.Conn
}

func (cr *CmdRunner) ReadReq() (*NmgrReq, error) {
	pkt, err := cr.conn.ReadPacket()
	if err != nil {
		return nil, err
	}

	nmr, err := DeserializeNmgrReq(pkt.GetBytes())
	if err != nil {
		return nil, err
	}

	return nmr, nil
}

func (cr *CmdRunner) WriteReq(nmr *NmgrReq) error {
	data := []byte{}

	data, err := nmr.SerializeRequest(data)
	if err != nil {
		return err
	}

	pkt, err := NewPacket(len(data))
	if err != nil {
		return err
	}

	pkt.AddBytes(data)

	if err := cr.conn.Write(pkt); err != nil {
		return err
	}

	return nil
}

func NewCmdRunner(conn *transport.Conn) (*CmdRunner, error) {
	cmd := &CmdRunner{
		conn: conn,
	}

	return cmd, nil
}