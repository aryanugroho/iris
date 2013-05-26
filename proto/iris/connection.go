// Iris - Distributed Messaging Framework
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// Iris is dual licensed: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// The framework is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
// more details.
//
// Alternatively, the Iris framework may be used in accordance with the terms
// and conditions contained in a signed written agreement between you and the
// author(s).
//
// Author: peterke@gmail.com (Peter Szilagyi)

package iris

import (
	"fmt"
	"github.com/karalabe/iris/proto/carrier"
	"sync"
	"time"
)

var appPrefix = "app:"
var topPrefix = "top:"

type connection struct {
	app   string              // Connection identifier
	relay *carrier.Connection // Message relay into the network

	reqIdx uint64                         // Index to assign the next request
	reqs   map[uint64]chan []byte         // Active requests waiting for a reply
	subs   map[string]SubscriptionHandler // Active subscriptions
	tunIdx uint64                         // Index to assign the next tunnel
	tuns   map[uint64]*tunnel             // Active tunnels

	hand ConnecionHandler
	lock sync.Mutex
}

func Connect(relay carrier.Carrier, app string, hand ConnecionHandler) Connection {
	// Create the new connection
	c := &connection{
		app:  app,
		reqs: make(map[uint64]chan []byte),
		subs: make(map[string]SubscriptionHandler),
		tuns: make(map[uint64]*tunnel),
		hand: hand,
	}
	c.relay = relay.Connect(c)
	c.relay.Subscribe(appPrefix + app)

	return c
}

// Implements iris.Connection.Request.
func (c *connection) Request(app string, msg []byte, timeout time.Duration) ([]byte, error) {
	// Create a reply channel for the results
	c.lock.Lock()
	reqChan := make(chan []byte, 1)
	reqId := c.reqIdx
	c.reqs[reqId] = reqChan
	c.reqIdx++
	c.lock.Unlock()

	// Make sure reply channel is cleaned up
	defer func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		delete(c.reqs, reqId)
		close(reqChan)
	}()
	// Send the request to the specified app
	c.relay.Balance(appPrefix+app, assembleRequest(reqId, msg))

	// Retrieve the results or time out
	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("iris: request timed out")
	case rep := <-reqChan:
		return rep, nil
	}
}

// Implements iris.Connection.Broadcast.
func (c *connection) Broadcast(app string, msg []byte) error {
	c.relay.Publish(appPrefix+app, assembleBroadcast(msg))
	return nil
}

// Implements iris.Connection.Subscribe.
func (c *connection) Subscribe(topic string, handler SubscriptionHandler) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.subs[topPrefix+topic]; ok {
		return fmt.Errorf("already subscribed")
	}
	c.subs[topPrefix+topic] = handler
	return c.relay.Subscribe(topPrefix + topic)
}

// Implements iris.Connection.Publish.
func (c *connection) Publish(topic string, msg []byte) error {
	c.relay.Publish(topPrefix+topic, assemblePublish(msg))
	return nil
}

// Implements iris.Connection.Unsubscribe.
func (c *connection) Unsubscribe(topic string) error {
	// Remove subscription if present
	c.lock.Lock()
	_, ok := c.subs[topPrefix+topic]
	delete(c.subs, topPrefix+topic)
	c.lock.Unlock()

	// Notify the carrier of the removal
	if ok {
		c.relay.Unsubscribe(topPrefix + topic)
	} else {
		return fmt.Errorf("not subscribed")
	}
	return nil
}

// Implements iris.Connection.Close.
func (c *connection) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Remove all subscriptions.
	for topic, _ := range c.subs {
		c.relay.Unsubscribe(topic)
	}
	c.relay.Unsubscribe(appPrefix + c.app)
}
