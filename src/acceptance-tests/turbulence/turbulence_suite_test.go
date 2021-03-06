package turbulence_test

import (
	"time"

	"github.com/cloudfoundry-incubator/consul-release/src/acceptance-tests/testing/helpers"
	"github.com/pivotal-cf-experimental/bosh-test/bosh"
	turbulenceclient "github.com/pivotal-cf-experimental/bosh-test/turbulence"
	"github.com/pivotal-cf-experimental/destiny/iaas"
	"github.com/pivotal-cf-experimental/destiny/turbulence"

	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestTurbulence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "turbulence")
}

var (
	config helpers.Config
	client bosh.Client

	turbulenceManifest turbulence.Manifest
	turbulenceClient   turbulenceclient.Client
)

var _ = BeforeSuite(func() {
	configPath, err := helpers.ConfigPath()
	Expect(err).NotTo(HaveOccurred())

	config, err = helpers.LoadConfig(configPath)
	Expect(err).NotTo(HaveOccurred())

	client = bosh.NewClient(bosh.Config{
		URL:              fmt.Sprintf("https://%s:25555", config.BOSH.Target),
		Username:         config.BOSH.Username,
		Password:         config.BOSH.Password,
		AllowInsecureSSL: true,
	})

	By("deploying turbulence", func() {
		info, err := client.Info()
		Expect(err).NotTo(HaveOccurred())

		guid, err := helpers.NewGUID()
		Expect(err).NotTo(HaveOccurred())

		manifestConfig := turbulence.Config{
			DirectorUUID: info.UUID,
			Name:         "turbulence-consul-" + guid,
			BOSH: turbulence.ConfigBOSH{
				Target:         config.BOSH.Target,
				Username:       config.BOSH.Username,
				Password:       config.BOSH.Password,
				DirectorCACert: config.BOSH.DirectorCACert,
			},
		}

		var iaasConfig iaas.Config
		switch info.CPI {
		case "aws_cpi":
			if config.AWS.Subnet == "" {
				Fail("aws.subnet is required for AWS IAAS deployment")
			}

			manifestConfig.IPRange = "10.0.4.0/24"
			iaasConfig = iaas.AWSConfig{
				AccessKeyID:           config.AWS.AccessKeyID,
				SecretAccessKey:       config.AWS.SecretAccessKey,
				DefaultKeyName:        config.AWS.DefaultKeyName,
				DefaultSecurityGroups: config.AWS.DefaultSecurityGroups,
				Region:                config.AWS.Region,
				Subnet:                config.AWS.Subnet,
				RegistryHost:          config.Registry.Host,
				RegistryPassword:      config.Registry.Password,
				RegistryPort:          config.Registry.Port,
				RegistryUsername:      config.Registry.Username,
			}
		case "warden_cpi":
			manifestConfig.IPRange = "10.244.4.0/24"
			iaasConfig = iaas.NewWardenConfig()
		default:
			Fail("unknown infrastructure type")
		}

		turbulenceManifest = turbulence.NewManifest(manifestConfig, iaasConfig)

		yaml, err := turbulenceManifest.ToYAML()
		Expect(err).NotTo(HaveOccurred())

		yaml, err = client.ResolveManifestVersions(yaml)
		Expect(err).NotTo(HaveOccurred())

		turbulenceManifest, err = turbulence.FromYAML(yaml)
		Expect(err).NotTo(HaveOccurred())

		err = client.Deploy(yaml)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() ([]bosh.VM, error) {
			return client.DeploymentVMs(turbulenceManifest.Name)
		}, "1m", "10s").Should(ConsistOf([]bosh.VM{
			{"running"},
		}))
	})

	By("preparing turbulence client", func() {
		turbulenceUrl := fmt.Sprintf("https://turbulence:%s@%s:8080",
			turbulenceManifest.Properties.TurbulenceAPI.Password,
			turbulenceManifest.Jobs[0].Networks[0].StaticIPs[0])

		turbulenceClient = turbulenceclient.NewClient(turbulenceUrl, 5*time.Minute, 2*time.Second)
	})
})

var _ = AfterSuite(func() {
	By("deleting the turbulence deployment", func() {
		if !CurrentGinkgoTestDescription().Failed {
			err := client.DeleteDeployment(turbulenceManifest.Name)
			Expect(err).NotTo(HaveOccurred())
		}
	})
})
