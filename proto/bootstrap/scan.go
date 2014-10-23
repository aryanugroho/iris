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

// Contains the address scanning ad-hoc seed generator. It continuously returns
// IP addresses up- and downwards from the current host address within the given
// network subnet.

package bootstrap

import (
	"fmt"
	"net"

	"gopkg.in/inconshreveable/log15.v2"
)

// Ad-hoc address scanning seed generator.
type scanSeeder struct {
	ipnet *net.IPNet
	quit  chan chan error
	log   log15.Logger
}

// Creates a new CoreOS seed generator.
func newScanSeeder(ipnet *net.IPNet, logger log15.Logger) (seeder, error) {
	return &scanSeeder{
		ipnet: ipnet,
		quit:  make(chan chan error),
		log:   logger.New("algo", "scan"),
	}, nil
}

// Starts the seed generator.
func (s *scanSeeder) Start(sink chan *net.IPAddr, phase *uint32) error {
	go s.run(sink, phase)
	return nil
}

// Terminates the seed generator.
func (s *scanSeeder) Close() error {
	errc := make(chan error)
	s.quit <- errc
	return <-errc
}

// Generates IP addresses in the network linearly from the current address.
func (s *scanSeeder) run(sink chan *net.IPAddr, phase *uint32) {
	s.log.Info("starting seed generator")
	var errc chan error
	var err error

	// Split the IP address into subnet and host parts
	subnetBits, maskBits := s.ipnet.Mask.Size()
	hostBits := maskBits - subnetBits

	hostIP := 0
	for i := 0; i < hostBits; i++ {
		hostIP += int(s.ipnet.IP[len(s.ipnet.IP)-1-i/8]) & (1 << uint(i%8))
	}
	// Make sure the specified IP net can be scanned (avoid point-to-point interfaces)
	if hostBits < 2 {
		err = fmt.Errorf("host address space too small: %v bits", hostBits)
	}
	// Loop until an error occurs or closure is requested
	for up, down, offset := true, true, 0; err == nil && errc == nil; {
		// If the address space was fully scanned, reset
		if !up && !down {
			up, down, offset = true, true, 0
		}
		// Generate the next host IP segment and update the offset
		nextIP := hostIP + offset
		offset = -offset
		if offset >= 0 {
			offset++
		}
		// Make sure we didn't run out of the subnet
		if nextIP <= 0 {
			down = false
			continue
		}
		if nextIP >= (1<<uint(hostBits))-1 {
			up = false
			continue
		}
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