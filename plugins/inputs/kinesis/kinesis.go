package kinesis

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/kinesis"
	consumer "github.com/harlow/kinesis-consumer"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
	"github.com/influxdata/telegraf/plugins/parsers"

	internalaws "github.com/influxdata/telegraf/internal/config/aws"
)

type (
	KinesisConsumer struct {
		Region      string `toml:"region"`
		AccessKey   string `toml:"access_key"`
		SecretKey   string `toml:"secret_key"`
		RoleARN     string `toml:"role_arn"`
		Profile     string `toml:"profile"`
		Filename    string `toml:"shared_credential_file"`
		Token       string `toml:"token"`
		EndpointURL string `toml:"endpoint_url"`
		StreamName  string `toml:"streamname"`

		consumer *consumer.Consumer
		parser   parsers.Parser
		ctx      context.Context
		cancel   context.CancelFunc
	}
)

var sampleConfig = `
  ## Amazon REGION of kinesis endpoint.
  region = "ap-southeast-2"

  ## Amazon Credentials
  ## Credentials are loaded in the following order
  ## 1) Assumed credentials via STS if role_arn is specified
  ## 2) explicit credentials from 'access_key' and 'secret_key'
  ## 3) shared profile from 'profile'
  ## 4) environment variables
  ## 5) shared credentials file
  ## 6) EC2 Instance Profile
  # access_key = ""
  # secret_key = ""
  # token = ""
  # role_arn = ""
  # profile = ""
  # shared_credential_file = ""

  ## Endpoint to make request against, the correct endpoint is automatically
  ## determined and this option should only be set if you wish to override the
  ## default.
  ##   ex: endpoint_url = "http://localhost:8000"
  # endpoint_url = ""

  ## Kinesis StreamName must exist prior to starting telegraf.
  streamname = "StreamName"

  ## Data format to consume.
  ## Each data format has its own unique set of configuration options, read
  ## more about them here:
  ## https://github.com/influxdata/telegraf/blob/master/docs/DATA_FORMATS_INPUT.md
  data_format = "influx"
`

func (k *KinesisConsumer) SampleConfig() string {
	return sampleConfig
}

func (k *KinesisConsumer) Description() string {
	return "Configuration for the AWS Kinesis input."
}

func (k *KinesisConsumer) SetParser(parser parsers.Parser) {
	k.parser = parser
}

func (k *KinesisConsumer) connect() error {
	credentialConfig := &internalaws.CredentialConfig{
		Region:      k.Region,
		AccessKey:   k.AccessKey,
		SecretKey:   k.SecretKey,
		RoleARN:     k.RoleARN,
		Profile:     k.Profile,
		Filename:    k.Filename,
		Token:       k.Token,
		EndpointURL: k.EndpointURL,
	}
	configProvider := credentialConfig.Credentials()
	client := kinesis.New(configProvider)

	_, err := client.DescribeStreamSummary(&kinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(k.StreamName),
	})

	consumer, err := consumer.New("telegraftest", consumer.WithClient(client))
	if err != nil {
		return err
	}

	k.consumer = consumer
	return nil
}

func (k *KinesisConsumer) Start(acc telegraf.Accumulator) error {
	if k.consumer == nil {
		err := k.connect()
		if err != nil {
			return err
		}
	}

	k.ctx, k.cancel = context.WithCancel(context.Background())

	go func() {
		err := k.consumer.Scan(k.ctx, func(r *consumer.Record) consumer.ScanStatus {
			k.onMessage(acc, r.Data)

			return consumer.ScanStatus{}
		})
		if err != nil {
			k.consumer = nil
		}
	}()

	return nil
}

func (k *KinesisConsumer) onMessage(acc telegraf.Accumulator, msg []byte) error {
	metrics, err := k.parser.Parse(msg)
	if err != nil {
		return err
	}

	for _, metric := range metrics {
		acc.AddMetric(metric)
	}

	return nil
}

func (k *KinesisConsumer) Stop() {
	k.cancel()
}

func (k *KinesisConsumer) Gather(acc telegraf.Accumulator) error {
	if k.consumer == nil {
		return k.connect()
	}

	return nil
}

func init() {
	inputs.Add("kinesis", func() telegraf.Input {
		return &KinesisConsumer{}
	})
}