@admin @priority=P3
Feature: Request Log
  As a developer using gmock
  I want to inspect and clear the request log via the admin API
  So that I can monitor incoming requests

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Request log returns all requests
    Given I register a stub via admin API for GET /api/log-test returning 200
    When I send a GET request to "/api/log-test"
    When I list request log
    Then the response status is 200

  @admin @priority=P3
  Scenario: Request log respects filter parameter
    Given I register a stub via admin API for GET /api/filter-test returning 200
    When I send a GET request to "/api/filter-test"
    When I list request log with filter "matched"
    Then the response status is 200

  @admin @priority=P3
  Scenario: Clear request log empties the log
    Given I register a stub via admin API for GET /api/clear-test returning 200
    When I send a GET request to "/api/clear-test"
    When I clear request log
    Then the response status is 204
    When I list request log
    Then the response status is 200
