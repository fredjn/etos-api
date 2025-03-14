// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package config

import (
	"flag"
	"os"
)

type SSEConfig interface {
	Config
	RabbitMQURI() string
}

type sseCfg struct {
	Config
	rabbitmqURI string
}

// NewSSEConfig creates a sse config interface based on input parameters or environment variables.
func NewSSEConfig() SSEConfig {
	var conf sseCfg

	flag.StringVar(&conf.rabbitmqURI, "rabbitmquri", os.Getenv("ETOS_RABBITMQ_URI"), "URI to the RabbitMQ ")
	base := load()
	conf.Config = base
	flag.Parse()
	return &conf
}

// RabbitMQURI returns the RabbitMQ URI.
func (c *sseCfg) RabbitMQURI() string {
	return c.rabbitmqURI
}
