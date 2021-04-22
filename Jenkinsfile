@Library('libpipelines@master') _

hose {
    EMAIL = 'eos@stratio.com'
    MODULE = 'oauth2-proxy'
    REPOSITORY = 'oauth2-proxy'
    PKGMODULESNAMES = ['oauth2-proxy']
    BUILDTOOL = 'make'
    NEW_VERSIONING = 'true'
    DEVTIMEOUT = 30
    ANCHORE_POLICY = "production"

    DEV = { config ->
        doUT(conf: config)
        doDocker(conf: config)
    }
}
