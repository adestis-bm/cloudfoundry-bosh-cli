package cmd_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	bmagentclient "github.com/cloudfoundry/bosh-micro-cli/agentclient"
	bmcmd "github.com/cloudfoundry/bosh-micro-cli/cmd"
	bmconfig "github.com/cloudfoundry/bosh-micro-cli/config"
	bmdepl "github.com/cloudfoundry/bosh-micro-cli/deployment"
	bmrel "github.com/cloudfoundry/bosh-micro-cli/release"
	bmretrystrategy "github.com/cloudfoundry/bosh-micro-cli/retrystrategy"
	bmstemcell "github.com/cloudfoundry/bosh-micro-cli/stemcell"

	fakesys "github.com/cloudfoundry/bosh-agent/system/fakes"
	fakebmcloud "github.com/cloudfoundry/bosh-micro-cli/cloud/fakes"
	fakecpideploy "github.com/cloudfoundry/bosh-micro-cli/cpideployer/fakes"
	fakebmdepl "github.com/cloudfoundry/bosh-micro-cli/deployment/fakes"
	fakemicrodeploy "github.com/cloudfoundry/bosh-micro-cli/microdeployer/fakes"
	fakebmstemcell "github.com/cloudfoundry/bosh-micro-cli/stemcell/fakes"
	fakeui "github.com/cloudfoundry/bosh-micro-cli/ui/fakes"
)

var _ = Describe("DeployCmd", func() {
	var (
		command                    bmcmd.Cmd
		userConfig                 bmconfig.UserConfig
		fakeFs                     *fakesys.FakeFileSystem
		fakeUI                     *fakeui.FakeUI
		fakeCpiDeployer            *fakecpideploy.FakeCpiDeployer
		logger                     boshlog.Logger
		release                    bmrel.Release
		fakeStemcellManager        *fakebmstemcell.FakeManager
		fakeStemcellManagerFactory *fakebmstemcell.FakeManagerFactory

		fakeMicroDeployer *fakemicrodeploy.FakeMicroDeployer

		fakeCpiManifestParser  *fakebmdepl.FakeManifestParser
		fakeBoshManifestParser *fakebmdepl.FakeManifestParser

		cpiReleaseTarballPath string
		stemcellTarballPath   string
		expectedStemcellCID   bmstemcell.CID
		expectedStemcell      bmstemcell.Stemcell
	)

	BeforeEach(func() {
		fakeUI = &fakeui.FakeUI{}
		fakeFs = fakesys.NewFakeFileSystem()
		userConfig = bmconfig.UserConfig{}
		fakeCpiDeployer = fakecpideploy.NewFakeCpiDeployer()
		fakeStemcellManager = fakebmstemcell.NewFakeManager()
		fakeStemcellManagerFactory = fakebmstemcell.NewFakeManagerFactory()

		fakeMicroDeployer = fakemicrodeploy.NewFakeMicroDeployer()

		fakeCpiManifestParser = fakebmdepl.NewFakeManifestParser()
		fakeBoshManifestParser = fakebmdepl.NewFakeManifestParser()

		logger = boshlog.NewLogger(boshlog.LevelNone)
		command = bmcmd.NewDeployCmd(
			fakeUI,
			userConfig,
			fakeFs,
			fakeCpiManifestParser,
			fakeBoshManifestParser,
			fakeCpiDeployer,
			fakeStemcellManagerFactory,
			fakeMicroDeployer,
			"fake-deployment-uuid",
			logger,
		)

		cpiReleaseTarballPath = "/release/tarball/path"

		stemcellTarballPath = "/stemcell/tarball/path"
		expectedStemcellCID = bmstemcell.CID("fake-stemcell-cid")
		expectedStemcell = bmstemcell.Stemcell{
			ImagePath:          "/stemcell/image/path",
			Name:               "fake-stemcell-name",
			Version:            "fake-stemcell-version",
			SHA1:               "fake-stemcell-sha1",
			RawCloudProperties: map[interface{}]interface{}{},
		}
	})

	Describe("Run", func() {
		It("returns err when no arguments are given", func() {
			err := command.Run([]string{})
			Expect(err).To(HaveOccurred())
			Expect(fakeUI.Errors).To(ContainElement("Invalid usage - deploy command requires exactly 2 arguments"))
		})

		It("returns err when 1 argument is given", func() {
			err := command.Run([]string{"something"})
			Expect(err).To(HaveOccurred())
			Expect(fakeUI.Errors).To(ContainElement("Invalid usage - deploy command requires exactly 2 arguments"))
		})

		It("returns err when 3 arguments are given", func() {
			err := command.Run([]string{"a", "b", "c"})
			Expect(err).To(HaveOccurred())
			Expect(fakeUI.Errors).To(ContainElement("Invalid usage - deploy command requires exactly 2 arguments"))
		})

		Context("when a CPI release is given", func() {
			BeforeEach(func() {
				fakeFs.WriteFileString(cpiReleaseTarballPath, "")
				fakeFs.WriteFileString(stemcellTarballPath, "")
			})

			Context("when there is a deployment set", func() {
				BeforeEach(func() {
					userConfig.DeploymentFile = "/some/deployment/file"

					// re-create command to update userConfig.DeploymentFile
					command = bmcmd.NewDeployCmd(
						fakeUI,
						userConfig,
						fakeFs,
						fakeCpiManifestParser,
						fakeBoshManifestParser,
						fakeCpiDeployer,
						fakeStemcellManagerFactory,
						fakeMicroDeployer,
						"fake-deployment-uuid",
						logger,
					)

					release = bmrel.Release{
						Name:          "fake-release",
						Version:       "fake-version",
						ExtractedPath: "/some/release/path",
						TarballPath:   cpiReleaseTarballPath,
					}

					releaseContents :=
						`---
name: fake-release
version: fake-version
`
					fakeFs.WriteFileString("/some/release/path/release.MF", releaseContents)
				})

				Context("when the deployment manifest exists", func() {
					var (
						boshDeployment bmdepl.Deployment
						cpiDeployment  bmdepl.Deployment
						cloud          *fakebmcloud.FakeCloud
					)
					BeforeEach(func() {
						fakeFs.WriteFileString(userConfig.DeploymentFile, "")
						cpiDeployment = bmdepl.Deployment{
							Registry: bmdepl.Registry{
								Username: "fake-username",
							},
							SSHTunnel: bmdepl.SSHTunnel{
								Host: "fake-host",
							},
							Mbus: "http://fake-mbus-endpoint",
						}
						fakeCpiManifestParser.SetParseBehavior(userConfig.DeploymentFile, cpiDeployment, nil)

						boshDeployment = bmdepl.Deployment{}
						fakeBoshManifestParser.SetParseBehavior(userConfig.DeploymentFile, boshDeployment, nil)
						cloud = fakebmcloud.NewFakeCloud()
						fakeCpiDeployer.SetDeployBehavior(cpiDeployment, cpiReleaseTarballPath, cloud, nil)
						fakeStemcellManagerFactory.SetNewManagerBehavior(cloud, fakeStemcellManager)

						fakeMicroDeployer.SetDeployBehavior(nil)
						fakeStemcellManager.SetUploadBehavior(stemcellTarballPath, expectedStemcell, expectedStemcellCID, nil)

						fakeFs.WriteFile(stemcellTarballPath, []byte{})
					})

					It("parses the CPI portion of the manifest", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).NotTo(HaveOccurred())
						Expect(fakeCpiManifestParser.ParseInputs[0].DeploymentPath).To(Equal("/some/deployment/file"))
					})

					It("parses the Bosh portion of the manifest", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).NotTo(HaveOccurred())
						Expect(fakeBoshManifestParser.ParseInputs[0].DeploymentPath).To(Equal("/some/deployment/file"))
					})

					It("deploys the CPI locally", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).NotTo(HaveOccurred())
						Expect(fakeCpiDeployer.DeployInputs[0].Deployment).To(Equal(cpiDeployment))
					})

					It("uploads the stemcell", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).NotTo(HaveOccurred())
						Expect(fakeStemcellManager.UploadInputs).To(Equal(
							[]fakebmstemcell.UploadInput{
								{
									TarballPath: stemcellTarballPath,
								},
							},
						))
					})

					It("creates a VM", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).NotTo(HaveOccurred())
						agentClient := bmagentclient.NewAgentClient("http://fake-mbus-endpoint", "fake-deployment-uuid", logger)
						agentPingRetryable := bmagentclient.NewPingRetryable(agentClient)
						expectedAgentPingRetryStrategy := bmretrystrategy.NewAttemptRetryStrategy(300, agentPingRetryable, logger)
						Expect(fakeMicroDeployer.DeployInput).To(Equal(
							fakemicrodeploy.DeployInput{
								Cloud:                  cloud,
								Deployment:             boshDeployment,
								StemcellCID:            expectedStemcellCID,
								Registry:               cpiDeployment.Registry,
								SSHTunnelConfig:        cpiDeployment.SSHTunnel,
								AgentPingRetryStrategy: expectedAgentPingRetryStrategy,
							},
						))
					})

					Context("when parsing the cpi deployment manifest fails", func() {
						It("returns error", func() {
							fakeCpiManifestParser.SetParseBehavior(userConfig.DeploymentFile, bmdepl.Deployment{}, errors.New("fake-parse-error"))

							err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("Parsing CPI deployment manifest"))
							Expect(err.Error()).To(ContainSubstring("fake-parse-error"))
							Expect(fakeCpiManifestParser.ParseInputs[0].DeploymentPath).To(Equal("/some/deployment/file"))
						})
					})

					Context("when reading stemcell fails", func() {
						It("returns error", func() {
							fakeStemcellManager.SetUploadBehavior(stemcellTarballPath, bmstemcell.Stemcell{}, bmstemcell.CID(""), errors.New("fake-reading-error"))

							err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("Uploading stemcell"))
							Expect(err.Error()).To(ContainSubstring("fake-reading-error"))
						})
					})
				})

				Context("when the deployment manifest is missing", func() {
					BeforeEach(func() {
						userConfig.DeploymentFile = "/some/deployment/file"

						// re-create command to update userConfig.DeploymentFile
						command = bmcmd.NewDeployCmd(
							fakeUI,
							userConfig,
							fakeFs,
							fakeCpiManifestParser,
							fakeBoshManifestParser,
							fakeCpiDeployer,
							fakeStemcellManagerFactory,
							fakeMicroDeployer,
							"fake-deployment-uuid",
							logger,
						)
					})

					It("returns err", func() {
						err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("Reading deployment manifest for deploy"))
						Expect(fakeUI.Errors).To(ContainElement("Deployment manifest path `/some/deployment/file' does not exist"))
					})
				})

			})

			Context("when there is no deployment set", func() {
				It("returns err", func() {
					err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
					Expect(err).To(HaveOccurred())
					Expect(fakeUI.Errors).To(ContainElement("No deployment set"))
				})
			})

			Context("When the CPI release tarball does not exist", func() {
				BeforeEach(func() {
					fakeFs.RemoveAll(cpiReleaseTarballPath)
				})

				It("returns error", func() {
					err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Checking CPI release `/release/tarball/path' existence"))
					Expect(fakeUI.Errors).To(ContainElement("CPI release `/release/tarball/path' does not exist"))
				})
			})

			Context("When the CPI stemcell tarball does not exist", func() {
				BeforeEach(func() {
					fakeFs.RemoveAll(stemcellTarballPath)
				})

				It("returns error", func() {
					err := command.Run([]string{cpiReleaseTarballPath, stemcellTarballPath})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Checking stemcell `/stemcell/tarball/path' existence"))
					Expect(fakeUI.Errors).To(ContainElement("Stemcell `/stemcell/tarball/path' does not exist"))
				})
			})
		})
	})
})
