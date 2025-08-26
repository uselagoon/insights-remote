# Insights Remote

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11021/badge)](https://www.bestpractices.dev/projects/11021)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/uselagoon/insights-remote)](https://securityscorecards.dev/viewer/?uri=github.com/uselagoon/insights-remote)
[![coverage](https://raw.githubusercontent.com/uselagoon/insights-remote/badges/.badges/main/coverage.svg)](https://github.com/uselagoon/insights-remote/actions/workflows/coverage.yaml)

The insights-remote system provides two separate paths to get insights data back to Lagoon Core.


### Insights via ConfigMaps

The first path through which data is fed into insights is via the creation of Kubernetes ConfigMaps.

When a Lagoon project is being built, the build-deploy tool will run -- see [this file](https://github.com/uselagoon/build-deploy-tool/blob/main/legacy/scripts/exec-generate-insights-configmap.sh) -- a Docker image inspect, as well as generating an SBOM using [Syft](https://github.com/anchore/syft).

These files are then added to Kubernetes configmaps that are given the following labels:
* lagoon.sh/insightsProcessed
  * This label is unset, to ensure it doesn’t exist
    * It is used by the insights-remote controller to mark an insights configMap has having been processed
  * lagoon.sh/insightsType=sbom-gz
    * The “insightsType” is used by the insights-remote service in core to determine what to do with the incoming data.
  * lagoon.sh/buildName=${LAGOON_BUILD_NAME}
    * Not currently used by insights, but is useful information to know which build process produced the insights artifact
  * lagoon.sh/project=${PROJECT}
    * Explicitly recording the Lagoon project
    * This information can be gathered from the k8s namespace as well
  * lagoon.sh/environment=${ENVIRONMENT}
    * Explicitly recording the Lagoon environment
  * lagoon.sh/service=${IMAGE_NAME}
    * This records which service’s container image this insights data was recorded for (eg, nginx, cli, solr, etc.)

Once the build-deploy tool has created the configMap, the insights-remote controller takes over, specifically [controllers/configmap_controller.go](https://github.com/anchore/syft)

This is a conceptually very simple controller, as far as Kubernetes controllers are concerned.

1. It monitors for the creation of any new ConfigMaps that have the label `lagoon.sh/insightsType` and which do NOT have a “lagoon.sh/insightsProcessed” label.
2. It then takes the configMap pushes all the data (payload, labels, annotations, etc.) into a LagoonInsightsMessage structure and pushes it to the Lagoon broker to the “lagoon-insights:items” queue
   * If pushing to the broker fails, we add a “lagoon.sh/insightsWriteDeferred” label with a time-after-which we should retry (5 minutes).
   * This insightsWriteDeferred label is used by the “insights deferred clear cron” [process](main.go#L366-L414) which simply removes the label after the appropriate date/time. Removing the label kicks off the process from point (1) above again.
3. Once this data has been pushed to the broker, the controller will label the configMap with “lagoon.sh/insightsProcessed”, as well as the date/time it was processed.

If the controller has been started with the `burn-after-reading` option (via `--burn-after-reading=true` or setting the environment variable `BURN_AFTER_READING=TRUE`), then any insights configMap that has a “lagoon.sh/insightsProcessed” label will be removed.

### Insights written directly to insights remote

The second approach to writing facts and problems back to core is via an HTTP post to insights-remote.

There are several parts to this - but from a user perspective it is fairly straightforward.

1. You grab your authentication token from `/var/run/secrets/lagoon/dynamic/insights-token/INSIGHTS_TOKEN`
2. You structure your facts or problems as an array of JSON objects
   * Facts: https://github.com/uselagoon/insights-remote/blob/main/internal/defs.go#L3-L14
   * Problems: https://github.com/uselagoon/insights-remote/blob/main/internal/defs.go#L3-L14
3. You POST your array of facts/problems
   * To the appropriate endpoint
     * `http://lagoon-remote-insights-remote.lagoon.svc/facts`
     * `http://lagoon-remote-insights-remote.lagoon.svc/problems`
   * With the token from step 1 in the “Authorization” header

How this all works is coordinated across a few subsystems.

#### Clearing facts and problems

There are cases where clearing facts/problems for an environment _without_ writing a new set of insights.

This is done by issuing a DELETE method call (with the authentication token in the header) to one of the following four endpoints

* `http://lagoon-remote-insights-remote.lagoon.svc/facts/{SOURCE}`
* `http://lagoon-remote-insights-remote.lagoon.svc/problems/{SOURCE}`
* `http://lagoon-remote-insights-remote.lagoon.svc/facts/{SOURCE}/{SERVICE}`
* `http://lagoon-remote-insights-remote.lagoon.svc/problems/{SOURCE}/{SERVICE}`

Where `SOURCE` and `SERVICE` target the appropriate categorizations used generating the insights.

Essentially these calls correspond to the "deleteProblemsFromSource" and "deleteFactsFromSource" Lagoon API calls

#### Authorization Token

The Authorization token is a JWT that is generated per project and environment by the insights-remote [namespace controller](internal/controllers/namespace_controller.go)

The rough idea here is that, given we have an http endpoint people can write to, we would like
1. To make writing to it as simple as possible - preferably just posting a list of facts or problems as a JSON
2. Prevent people from willy-nilly being able to write data about namespaces they actually don’t have access to

In order to make this possible, we generate a signed JWT that is injected into a project’s containers that uniquely identify it. If our service gets a valid token, the only way it could’ve been acquired is through access to the environments that it is valid for.


The process is, roughly:
1. The controller goes through all namespaces looking for lagoon environments
2. For each of those it finds, if there is no secret labelled `lagoon.sh/insights-token` it will then
   1. Generate a JWT codifying the project and environment
   2. Create a secret with the labels
      * `lagoon.sh/insights-token`
      * `lagoon.sh/dynamic-secret=insights-token` which will mark a secret as needing to be dynamically loaded (see https://github.com/uselagoon/remote-controller/pull/207)
3. This secret then shows up in the containers at `/var/run/secrets/lagoon/dynamic/insights-token/INSIGHTS_TOKEN`

#### Endpoints

The two endpoints for facts and problems are simple golang Gin routes that expect
1. An “Authorization” header containing the auth jwt as its value
2. A JSON payload of arrays of facts/problems, as described above.

These two routes simply unmarshal the data, use the authorization header to set the project/environment details, and then send along these “direct” problems or facts to the insights-handler in Lagoon core for further processing.

## Dependency Track Integration

Insights remote allows integration into [Dependency Track](https://docs.dependencytrack.org/) - specifically, the uploading of SBOMS.

Once a build is complete and the SBOM has been sent to Insights Core, a post-processing step is triggered and pushes the SBOM to Dependency Track.

Dependency track integration is enabled by starting Insights Remote with the appropriate flags or the following environment variables in the insights-remote deployment:
* ENABLE_DEPENDENCY_TRACK_INTEGRATION=true
* DEPENDENCY_TRACK_API_ENDPOINT=https://dependency-track.example.com
* DEPENDENCY_TRACK_API_KEY=your-api-key

# Templates

The Dependency Track integration allows a fair amount of customization in how the SBOM is uploaded and displayed hierarchically in Dependency Track.
This is accomplished by setting templates for the Project hierarchy.

By default we have
- Project - default template=`{{ .ProjectName }}`
  - Environment - default template=`{{ .ProjectName }}-{{ .EnvironmentName }}`
    - Service - default template=`{{ .ProjectName }}-{{ .EnvironmentName }}-{{ .ServiceName }}`

These templates then set the Project hierarchy in Dependency Track.
The following variables are available for use in the templates:
- .ProjectName
- .EnvironmentName
- .ServiceName
- .EnvironmentType

These are currently set by overriding the arguments in the insights-remote deployment.

