# Neutron - a lightweight pipeline system based on Kubernetes

## Architecture overview

```plantuml
frame "neutron system" {
database ps as "pipeline states"
component server as "neutron server"
}
cloud cluster as "kubernetes cluster" {
    node job1 as "pipeline job"
    node job2 as "pipeline job"
    node job3 as "pipeline job"
}
frame codebase {
component codebase1 as "code base"
component codebase2 as "code base"
}
codebase1 -down-> server : webhook
codebase2 -down-> server : webhook
server <-> ps
job1 -up-> codebase1 : "report states"
job2 -up-> codebase1 : "report states"
job3 -up-> codebase2 : "report states"
server -down-> job1: start
server -down-> job2: start
server -down-> job3: start
```

Neutron system contains a stateless web api server with a database.
Codebase like GitLab or Gogs send pipeline request to api server via webhooks. 
Neutron api server parse these webhook requests, then call Kubernetes api to start a [job](https://kubernetes.io/docs/concepts/workloads/controllers/job/) in order to process all pipeline logic.
After Kubernetes job completed, job will report status to codebase itself.

## Install

## Usage
