package sqsclient

import (
	"context"
	"fmt"

	appconfig "agent-client-go/internal/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSClient expõe os métodos ReceiveMessage, SendMessage e DeleteMessage do SDK AWS SQS.
type SQSClient interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type AWSSQS struct {
	client *sqs.Client
}

func NewAWSSQS(ctx context.Context) (*AWSSQS, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(appconfig.GetString("AWS_REGION")),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao carregar config aws: %w", err)
	}

	return &AWSSQS{client: sqs.NewFromConfig(awsCfg)}, nil
}

func (a *AWSSQS) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return a.client.ReceiveMessage(ctx, params, optFns...)
}

func (a *AWSSQS) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	return a.client.SendMessage(ctx, params, optFns...)
}

func (a *AWSSQS) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	return a.client.DeleteMessage(ctx, params, optFns...)
}
