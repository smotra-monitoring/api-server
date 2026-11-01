#!/bin/bash

curl -X POST api.smotra.net:8080/v1/agent/claim -d '{"agent_id":"019d18de-9f15-73b0-9ee5-380b78983e3c", "claim_token":"eIgXU4KVSGEshHiKppYToFsDfvFtBcfhkaZJu5qEEDukXM4K0G3PLLWnn2DzfhhR","name":"localhost"}' -H "Content-Type: application/json"
