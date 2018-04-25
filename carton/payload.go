/*
** Copyright [2013-2017] [Megam Systems]
**
** Licensed under the Apache License, Version 2.0 (the "License");
** you may not use this file except in compliance with the License.
** You may obtain a copy of the License at
**
** http://www.apache.org/licenses/LICENSE-2.0
**
** Unless required by applicable law or agreed to in writing, software
** distributed under the License is distributed on an "AS IS" BASIS,
** WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
** See the License for the specific language governing permissions and
** limitations under the License.
 */
package carton

import (
	"encoding/json"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/virtengine/libgo/api"
)

type Payload struct {
	Id        string    `json:"id"`
	Action    string    `json:"action"`
	CatId     string    `json:"cat_id"`     // Cartons Id
	AccountId string    `json:"account_id"` // Account Id
	CatType   string    `json:"cattype"`    // Cartons Type
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
}

type PayloadConvertor interface {
	Convert(p *Payload) (*Requests, error)
}

// NewPayload decode the Payload from given raw NSQ JSON message
func NewPayload(b []byte) (*Payload, error) {
	p := &Payload{}
	err := json.Unmarshal(b, &p)
	if err != nil {
		return nil, err
	}
	return p, err
}

// Convert will convert the NSQ Payload to the API Requests struct
// If the 'CatId' is invalid (len() < 10), it will call Vertice API "/requests/:id" to retrieve the Requests list.
// Otherwise, it will use the given information from the Payload to create a Requests object.
func (p *Payload) Convert() (*Requests, error) {
	if len(strings.TrimSpace(p.CatId)) < 10 {
		return listReqsById(p.Id, p.AccountId)
	} else {
		return &Requests{
			Action:    p.Action,
			Category:  p.Category,
			AccountId: p.AccountId,
			CatId:     p.CatId,
			CreatedAt: p.CreatedAt,
		}, nil
	}

}

//The payload in the queue can be just a pointer or a value.
//pointer means just the id will be available and rest is blank.
//value means the id is blank and others are available.
func listReqsById(id, email string) (*Requests, error) {
	log.Debugf("list requests %s", id)
	cl := api.NewClient(newArgs(email, ""), "/requests/"+id)
	response, err := cl.Get()
	if err != nil {
		return nil, err
	}

	res := &ApiRequests{}
	err = json.Unmarshal(response, res)
	if err != nil {
		return nil, err
	}
	r := &res.Results[0]
	log.Debugf("Requests %v", r)
	return r, nil
}
