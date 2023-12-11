package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRegionFromAwsSmArn(t *testing.T) {
	region, ok := getAwsSmSecretRegion("arn:aws:secretsmanager:ap-southeast-2:1234567:secret:my-global-secret")
	assert.True(t, ok)
	assert.Equal(t, "ap-southeast-2", region)
}

func TestParseRegionFromAwsSmId(t *testing.T) {
	_, ok := getAwsSmSecretRegion("just-an-id")
	assert.False(t, ok)
}
