@admin @priority=P3
Feature: Metrics
  As a developer using gmock
  I want to inspect server metrics via Prometheus and JSON endpoints
  So that I can monitor mock server behavior

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Prometheus metrics endpoint returns text format
    When I fetch Prometheus metrics
    Then the response status is 200
    Then the Prometheus response contains "gochaos_"

  @admin @priority=P3
  Scenario: JSON metrics endpoint returns all metric keys
    When I fetch JSON metrics
    Then the response status is 200
    Then the JSON metrics contain "requests_total"
    Then the JSON metrics contain "requests_matched"
    Then the JSON metrics contain "requests_unmatched"
    Then the JSON metrics contain "faults_injected"
    Then the JSON metrics contain "faults_delayed"
    Then the JSON metrics contain "stubs_registered"
    Then the JSON metrics contain "admin_operations"
    Then the JSON metrics contain "nearmiss_queries"

  @admin @priority=P3
  Scenario: Counters increment after relevant events
    Given I register a stub via admin API for GET /api/metrics-counts returning 200
    When I fetch JSON metrics
    Then the JSON metrics contain "stubs_registered"
    Then the JSON metrics contain "admin_operations"
