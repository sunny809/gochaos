@admin @priority=P3
Feature: Health Endpoints
  As a developer using gmock
  I want to check server health via liveness and readiness probes
  So that I can monitor server status in Kubernetes environments

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Liveness endpoint returns alive status
    When I check liveness endpoint
    Then the response status is 200

  @admin @priority=P3
  Scenario: Readiness endpoint returns ready status
    Given I register a stub via admin API for GET /api/ready-test returning 200
    When I check readiness endpoint
    Then the response status is 200

  @admin @priority=P3
  Scenario: Readiness reflects stub count
    Given I register a stub via admin API for GET /api/health-a returning 200
    When I check readiness endpoint
    Then the response status is 200
    When I register a stub via admin API for GET /api/health-b returning 201
    When I check readiness endpoint
    Then the response status is 200
