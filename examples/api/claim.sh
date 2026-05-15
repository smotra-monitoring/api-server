#!/bin/bash

curl -X POST api.smotra.net:8080/v1/agent/claim -d '{"agentId":"019d18de-9f15-73b0-9ee5-380b78983e3c", "claimToken":"eIgXU4KVSGEshHiKppYToFsDfvFtBcfhkaZJu5qEEDukXM4K0G3PLLWnn2DzfhhR","sectionId":"019d18bf-d0a6-7052-8c59-8110417675f1","name":"localhost"}' -H "Content-Type: application/json"
