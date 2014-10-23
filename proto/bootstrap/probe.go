// Iris - Decentralized cloud messaging
// Copyright (c) 2014 Project Iris. All rights reserved.
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

// Contains the random address probing ad-hoc seed generator. It continuously
// returns IP addresses randomly from the current host address within the given
// network subnet.

package bootstrap

import (
	"fmt"
	"math/rand"
	"net"

	"gopkg.in/inconshreveable/log15.v2"
)

// Ad-hoc address scanning seed generator.
type probeSeeder struct {
	ipnet *net.IPNet
	quit  chan chan error
	log   log15.Logger
}

// Creates a new probing seed generator.
func newProbeSeeder(ipnet *net.IPNet, logger log15.Logger) (seeder, error) {
	return &probeSeeder{
		ipnet: ipnet,
		quit:  make(chan chan error),
		log:   logger.New("algo", "probe"),
	}, nil
}

// Starts the seed generator.
func (s *probeSeeder) Start(sink chan *net.IPAddr, phase *uint32) error {
	go s.run(sink, phase)
	return nil
}

// Terminates the seed generator.
func (s *probeSeeder) Close() error {
	errc := make(chan error)
	s.quit <- errc
	return <-errc
}

// Generates IP addresses in the network linearly from the current address.
func (s *probeSeeder) run(sink chan *net.IPAddr, phase *uint32) {
	s.log.Info("starting seed generator")
	var errc chan error
	var err error

	// Split the IP address into subnet and host parts
	subnetBits, maskBits := s.ipnet.Mask.Size()
	hostBits := maskBits - subnetBits

	// Make sure the specified IP net can be probed (avoid point-to-point interfaces)
	if hostBits < 2 {
		err = fmt.Errorf("host address space too small: %v bits", hostBits)
	}
	// Loop until an error occurs or closure is requested
	for err == nil && errc == nil {
		// Generate a random IP address within the subnet (ignore subnet and broadcast address)
		nextIP := rand.Intn(1<<uint(hostBits)-2) + 1

		// Generate the full host address and send it upstream
		host := s.ipnet.IP.Mask(s.ipnet.Mask)
		for i := len(host) - 1; i >= 0; i-- {
			host[i] |= byte(nextIP & 255)
			nextIP >>= 8
		}
		select {
		case sink <- &net.IPAddr{IP: host}:
		case errc = <-s.quit:
		}
	}
	// Log termination status, wait until closure request and return
	if err != nil {
		s.log.Error("seeder terminating prematurely", "error", err)
	} else {
		s.log.Info("seeder terminating gracefully")
	}
	if errc == nil {
		errc = <-s.quit
	}
	errc <- err
}