package lambda

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/AlekSi/pointer"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"log/slog"
	"os"
	"testing"
	"time"
)

const region = "eu-central-1"

var _ctx = context.Background()

// https://docs.localstack.cloud/user-guide/aws/lambda/
func TestLambdaClient(t *testing.T) {
	endpoint, err := startLocalstack(t)
	require.NoError(t, err)

	awsCli, err := createClient(endpoint)
	require.NoError(t, err)

	functionARN, err := createLambda(awsCli, "my-function")
	require.NoError(t, err)

	lambdaCli, err := New(awsCli, functionARN)
	require.NoError(t, err)

	body := []byte(`{"key":"value"}`)
	response, err := lambdaCli.Invoke(_ctx, "POST", "/path", body)
	require.NoError(t, err)

	assert.Equal(t, "Hello from Lambda!", response)
}

func startLocalstack(t *testing.T) (string, error) {
	// https://golang.testcontainers.org/modules/localstack/
	container, err := localstack.Run(_ctx, "localstack/localstack:latest", testcontainers.WithEnv(map[string]string{
		"AWS_ACCESS_KEY_ID":     "test",
		"AWS_SECRET_ACCESS_KEY": "test",
		"AWS_REGION":            region,
	}))
	if err != nil {
		return "", fmt.Errorf("localstack.Run: %w", err)
	}
	testcontainers.CleanupContainer(t, container)

	endpoint, err := container.PortEndpoint(_ctx, "4566/tcp", "http")
	if err != nil {
		return "", fmt.Errorf("container.PortEndpoint: %w", err)
	}

	return endpoint, nil
}

func createClient(endpoint string) (*lambda.Client, error) {
	cfg, err := config.LoadDefaultConfig(_ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	if err != nil {
		return nil, fmt.Errorf("config.LoadDefaultConfig: %w", err)
	}

	cli := lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.BaseEndpoint = pointer.To(endpoint)
	})

	return cli, nil
}

func createLambda(cli *lambda.Client, functionName string) (string, error) {
	buf, err := zipBuffer("index.py")
	if err != nil {
		return "", fmt.Errorf("zipBuffer: %w", err)
	}

	resp, err := cli.CreateFunction(_ctx, &lambda.CreateFunctionInput{
		FunctionName: pointer.ToString(functionName),
		Runtime:      types.RuntimePython38,
		// LocalStack does not enforce IAM policies, so one can use any ARN format role.
		Role:    pointer.ToString("arn:aws:iam::000000000000:role/lambda-role"),
		Handler: pointer.ToString("index.handler"),
		Code: &types.FunctionCode{
			ZipFile: buf.Bytes(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("cli.CreateFunction: %w", err)
	}

	if err := waitForFunction(cli, functionName); err != nil {
		return "", fmt.Errorf("waitForFunction: %w", err)
	}

	return pointer.GetString(resp.FunctionArn), nil
}

func waitForFunction(cli *lambda.Client, functionName string) error {
	for {
		resp, err := cli.GetFunction(_ctx, &lambda.GetFunctionInput{
			FunctionName: pointer.ToString(functionName),
		})
		if err != nil {
			return fmt.Errorf("cli.GetFunction: %w", err)
		}

		if resp.Configuration != nil {
			switch resp.Configuration.State {
			case types.StateActive:
				return nil
			case types.StateFailed:
				return fmt.Errorf("cli.GetFunction: %s", pointer.GetString(resp.Configuration.StateReason))
			case types.StatePending:
				slog.Info("Function is Pending state")
			}
		}

		time.Sleep(1 * time.Second)
	}
}

func zipBuffer(fileName string) (*bytes.Buffer, error) {
	testDataFile := fmt.Sprintf("testdata/%s", fileName)

	functionCode, err := os.ReadFile(testDataFile)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadFile: %w", err)
	}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	fileWriter, err := zipWriter.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("zw.Create: %w", err)
	}

	if _, err := fileWriter.Write(functionCode); err != nil {
		return nil, fmt.Errorf("fw.Write: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("zw.Close: %w", err)
	}

	return &buf, nil
}

func TestMain(m *testing.M) {
	// Disable Ryuk
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// try
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_REGION", "us-east-1")

	code := m.Run()

	os.Exit(code)
}
