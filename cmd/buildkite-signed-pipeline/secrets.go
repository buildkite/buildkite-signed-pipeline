package main

import (
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

func getAwsSmSecretRegion(secretId string) (string, bool) {
	re := regexp.MustCompile("^arn:aws:secretsmanager:([^:]+):")
	result := re.FindStringSubmatch(secretId)
	if result == nil {
		return "", false
	}
	return result[1], true
}

func GetAwsSmSecret(secretId string) (string, error) {
	var awsSession *session.Session

	// use the ARN as a hint for the region of the secret rather than the default
	// this is because the region in the ARN means nothing to AWS SM
	if secretRegion, hasRegion := getAwsSmSecretRegion(secretId); hasRegion {
		awsSession = session.Must(session.NewSession(&aws.Config{Region: aws.String(secretRegion)}))
	} else {
		awsSession = session.Must(session.NewSession())
	}

	client := secretsmanager.New(awsSession)
	request := &secretsmanager.GetSecretValueInput {
		SecretId: aws.String(secretId),
	}

	result, err := client.GetSecretValue(request)
	if err != nil {
		return "", err
	}
	return *result.SecretString, nil
}
