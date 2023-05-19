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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/pubsub"
	"github.com/stretchr/testify/assert"
)

func TestParseNSQMetadata(t *testing.T) {
	t.Run("metadata is correct", func(t *testing.T) {
		fakeProperties := map[string]string{
			nsqlookupAddr:   "10.42.0.10:4160,10.42.0.20:4160",
			nsqdAddr:        "10.42.0.10:4150,10.42.0.20:4150",
			"read_timeout":  "30s",
			"write_timeout": "30s",
			"max_in_flight": "200",
		}
		fakeMetaData := pubsub.Metadata{
			Base: metadata.Base{
				Properties: fakeProperties,
			},
		}

		// paser metadata
		m, err := parseNSQMetadata(fakeMetaData)

		// assert
		assert.NoError(t, err)
		assert.NotEmpty(t, m.lookupdAddrs)
		lookups := strings.Join(m.lookupdAddrs, ",")
		assert.Equal(t, fakeProperties[nsqlookupAddr], lookups)

		assert.NotEmpty(t, m.addrs)
		nsqds := strings.Join(m.addrs, ",")
		assert.Equal(t, fakeProperties[nsqdAddr], nsqds)

		//nsqconfig value
		readtTimeOut, _ := time.ParseDuration(fakeProperties["read_timeout"])
		assert.Equal(t, readtTimeOut, m.config.ReadTimeout)
		writeTimeOut, _ := time.ParseDuration(fakeProperties["write_timeout"])
		assert.Equal(t, writeTimeOut, m.config.ReadTimeout)
		maxInFlight, _ := strconv.ParseInt(fakeProperties["max_in_flight"], 10, 32)
		assert.Equal(t, int(maxInFlight), m.config.MaxInFlight)
	})

	t.Run("subscribe metadata is correct", func(t *testing.T) {
		fakeProperties := map[string]string{
			concurrency: "4",
			channel:     "nsqchannel",
		}

		// paser metadata
		count, chname := parseSubMetadata(fakeProperties)

		// assert
		assert.Equal(t, strconv.Itoa(count), fakeProperties[concurrency])
		assert.Equal(t, chname, fakeProperties[channel])
	})
}
