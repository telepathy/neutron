# Neutron - a lightweight pipeline system based on Kubernetes

## Architecture overview

![Concept Arch](./cmd/api/static/arch.svg)

Neutron system contains a stateless web api server with a database.
Codebase like GitLab or Gogs send pipeline request to api server via webhooks. 
Neutron api server parse these webhook requests, then call Kubernetes api to start a [job](https://kubernetes.io/docs/concepts/workloads/controllers/job/) in order to process all pipeline logic.
After Kubernetes job completed, job will report status to codebase itself.

## Install

## Usage
