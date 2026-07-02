@admin @priority=P3
Feature: Near-Miss Diagnostics
  As a developer using gmock
  I want to query near-miss diagnostics for unmatched requests
  So that I can debug why a request did not match any stub

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Near-miss returns results for unmatched request
    Given I register a stub via admin API for GET /api/users returning 200
    When I query near-miss for GET "/api/users/123"
    Then the response status is 200
