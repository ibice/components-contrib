/*
Copyright 2023 The Dapr Authors
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
package nsq

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/pubsub"
	"github.com/dapr/kit/logger"

	gnsq "github.com/nsqio/go-nsq"
)

const (
	nsqlookupAddr = "nsqlookup"
	nsqdAddr      = "nsqd"
	channel       = "channel"

	concurrency = "concurrency"
)

type nsqPubSub struct {
	metadata nsqMetadata
	pubs     []*gnsq.Producer
	subs     []*subscriber

	logger logger.Logger
}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

// NewNSQPubSub returns a new NSQ pub-sub implementation
func NewNSQPubSub(logger logger.Logger) pubsub.PubSub {
	return &nsqPubSub{logger: logger}
}

func parseNSQMetadata(meta pubsub.Metadata) (nsqMetadata, error) {
	m := nsqMetadata{
		config: gnsq.NewConfig(),
	}

	if val, ok := meta.Properties[nsqlookupAddr]; ok && val != "" {
		m.lookupdAddrs = strings.Split(val, ",")
	}

	if val, ok := meta.Properties[nsqdAddr]; ok && val != "" {
		m.addrs = strings.Split(val, ",")
	} else {
		return m, errors.New("nsq error: missing nsqd Address")
	}

	for k, v := range meta.Properties {
		m.config.Set(k, v)
	}

	return m, nil
}

func parseSubMetadata(datas map[string]string) (int, string) {
	concurrencys := 1
	if val, ok := datas[concurrency]; ok && val != "" {
		if v, err := strconv.ParseUint(val, 10, 32); err == nil {
			concurrencys = int(v)
		}
	}

	host, _ := os.Hostname()
	chname := fmt.Sprintf("%s%s", host, "#ephemeral")
	if val, ok := datas[channel]; ok && val != "" {
		chname = val
	}
	return concurrencys, chname
}

func (n *nsqPubSub) Init(ctx context.Context, nsqMetadata pubsub.Metadata) error {
	meta, err := parseNSQMetadata(nsqMetadata)
	if err != nil {
		return err
	}

	n.metadata = meta

	producers := make([]*gnsq.Producer, 0, len(n.metadata.addrs))

	// create producers
	for _, addr := range n.metadata.addrs {
		p, err := gnsq.NewProducer(addr, n.metadata.config)
		if err != nil {
			return err
		}
		if err = p.Ping(); err != nil {
			return err
		}
		producers = append(producers, p)

		n.logger.Debugf("connected to nsq producer at %s", addr)
	}

	n.pubs = producers
	return nil
}

func (n *nsqPubSub) Publish(ctx context.Context, req *pubsub.PublishRequest) error {
	n.logger.Debugf("[nsq] publish to %s", req.Topic)
	p := n.pubs[rnd.Intn(len(n.pubs))]
	err := p.Publish(req.Topic, req.Data)
	if err != nil {
		return fmt.Errorf("nsq: error from publish: %s", err)
	}
	return nil
}

func (n *nsqPubSub) Subscribe(ctx context.Context, req pubsub.SubscribeRequest, handler pubsub.Handler) error {
	concurrencys, chname := parseSubMetadata(req.Metadata)
	c, err := gnsq.NewConsumer(req.Topic, chname, n.metadata.config)
	if err != nil {
		return err
	}

	h := gnsq.HandlerFunc(func(nm *gnsq.Message) error {
		n.logger.Debugf("[nsq] recevied msg %s on %s", nm.ID, req.Topic)
		return handler(ctx, &pubsub.NewMessage{Topic: req.Topic, Data: nm.Body})
	})

	c.AddConcurrentHandlers(h, concurrencys)

	if len(n.metadata.lookupdAddrs) > 0 {
		err = c.ConnectToNSQLookupds(n.metadata.lookupdAddrs)
		n.logger.Debugf("connected to nsq consumer at %s", n.metadata.lookupdAddrs)
	} else {
		err = c.ConnectToNSQDs(n.metadata.addrs)
		n.logger.Debugf("connected to nsq consumer at %s", n.metadata.addrs)
	}
	if err != nil {
		return err
	}

	sub := &subscriber{
		c:     c,
		topic: req.Topic,
		h:     h,
		n:     concurrencys,
	}

	n.subs = append(n.subs, sub)

	return nil
}

func (n *nsqPubSub) Close() error {
	// stop the producers
	for _, p := range n.pubs {
		p.Stop()
	}

	// stop the consumers
	for _, c := range n.subs {
		c.c.Stop()

		if len(n.metadata.lookupdAddrs) > 0 {
			// disconnect from all lookupd
			for _, addr := range n.metadata.lookupdAddrs {
				c.c.DisconnectFromNSQLookupd(addr)
			}
		} else {
			// disconnect from all nsq brokers
			for _, addr := range n.metadata.addrs {
				c.c.DisconnectFromNSQD(addr)
			}
		}
	}

	return nil
}

func (n *nsqPubSub) Features() []pubsub.Feature {
	return nil
}

// GetComponentMetadata returns the metadata of the component.
func (n *nsqPubSub) GetComponentMetadata() map[string]string {
	metadataStruct := nsqMetadata{}
	metadataInfo := map[string]string{}
	metadata.GetMetadataInfoFromStructType(reflect.TypeOf(metadataStruct), &metadataInfo, metadata.PubSubType)
	return metadataInfo
}
