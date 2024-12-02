# AWS Lambda Invoker

This repository contains an AWS Lambda invocation wrapper written in Go.

It also includes an integration test using TestContainers and Localstack.

## Features

- Invokes AWS Lambda functions synchronously and asynchronously.
- Wraps input body in `APIGatewayProxyRequest` and extracts output body from `APIGatewayProxyResponse`.
- Uses TestContainers and Localstack for integration testing.

### Prerequisites
A container runtime is required to run the integration tests, i.e.
- Docker
- Podman
- Colima

### Running Tests

To run the tests locally, use the following command:

```sh
make test
```

### Troubleshooting

`localstack.Run` under the hood binds Docker socket to the container to allow Localstack container to spawn child containers to execute Lambda functions code there.
Make sure that the Docker socket has appropriate permissions to be accessible by the Localstack container.

```go
    dockerHost := testcontainers.MustExtractDockerSocket(ctx)
    
    req := testcontainers.ContainerRequest{
        HostConfigModifier: func(hostConfig *container.HostConfig) {
            hostConfig.Binds = []string{fmt.Sprintf("%s:/var/run/docker.sock", dockerHost)}
        },
    }
```

## License

This project is licensed under the MIT License.