@Library('libpipelines') _

hose {
    EMAIL = 'eos@stratio.com'
    BUILDTOOL_IMAGE = 'golang:1.16'
    BUILDTOOL = 'make'
    DEVTIMEOUT = 30
    ANCHORE_POLICY = "production"

    DEV = { config ->
        doUT(conf: config, parameters: "HOME=/go")
        doDocker(conf: config, image: 'oauth2-proxy')
    }
}
