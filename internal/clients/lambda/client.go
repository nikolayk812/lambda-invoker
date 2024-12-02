package lambda

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AlekSi/pointer"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"net/http"
)

//go:generate mockgen -destination=./client_mock.go -package=lambda -mock_names Client=MockClient . Client
type Client interface {
	Invoke(ctx context.Context, httpMethod, path string, body []byte) (string, error)
	InvokeAsync(ctx context.Context, httpMethod, path string, body []byte) error
}

type client struct {
	cli         *lambda.Client
	functionARN string
}

func New(cli *lambda.Client, functionARN string) (Client, error) {
	if cli == nil {
		return nil, fmt.Errorf("lambda.NewFromConfig returned nil")
	}

	if _, err := arn.Parse(functionARN); err != nil {
		return nil, fmt.Errorf("arn.Parse[%s]: %w", functionARN, err)
	}

	return &client{
		cli:         cli,
		functionARN: functionARN,
	}, nil
}

// Invoke synchronously invokes the Lambda function with the given HTTP method and body.
// input body is wrapped in APIGatewayProxyRequest
// output body is extracted from APIGatewayProxyResponse
func (c *client) Invoke(ctx context.Context, httpMethod, path string, body []byte) (string, error) {
	out, err := c.invoke(ctx, false, httpMethod, path, body)
	if err != nil {
		return "", fmt.Errorf("invoke[sync]: %w", err)
	}

	return out, nil
}

func (c *client) InvokeAsync(ctx context.Context, httpMethod, path string, body []byte) error {
	if _, err := c.invoke(ctx, false, httpMethod, path, body); err != nil {
		return fmt.Errorf("invoke[async]: %w", err)
	}

	return nil
}

func (c *client) invoke(ctx context.Context, async bool, httpMethod, path string, body []byte) (out string, err error) {
	req := events.APIGatewayProxyRequest{
		Path:       path,
		HTTPMethod: httpMethod,
		Body:       string(body),
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("json.Marshal: %w", err)
	}

	invocationType := types.InvocationTypeRequestResponse
	if async {
		invocationType = types.InvocationTypeEvent
	}

	output, err := c.cli.Invoke(ctx, &lambda.InvokeInput{
		FunctionName:   pointer.To(c.functionARN),
		InvocationType: invocationType,
		LogType:        types.LogTypeNone,
		Payload:        payload,
	})
	if err != nil {
		return "", fmt.Errorf("cli.Invoke: %w", err)
	}

	if output == nil {
		return "", fmt.Errorf("output is nil")
	}

	if output.FunctionError != nil {
		return "", fmt.Errorf("output.FunctionError: %s", *output.FunctionError)
	}

	expectedStatus := http.StatusOK
	if async {
		expectedStatus = http.StatusAccepted
	}

	if output.StatusCode != int32(expectedStatus) {
		return "", fmt.Errorf("output.StatusCode: %d", output.StatusCode)
	}

	if async {
		if len(output.Payload) != 0 {
			return "", fmt.Errorf("output.Payload is not empty for async invocation: [%s]", output.Payload)
		}
		return "", nil
	}

	// sync invocation continues here
	if len(output.Payload) == 0 {
		return "", fmt.Errorf("output.Payload is empty for sync invocation")
	}

	var r events.APIGatewayProxyResponse
	if err := json.Unmarshal(output.Payload, &r); err != nil {
		return "", fmt.Errorf("json.Unmarshal: %w", err)
	}

	if r.StatusCode != http.StatusOK {
		return "", fmt.Errorf("response statusCode: %d", r.StatusCode)
	}

	return r.Body, nil
}
