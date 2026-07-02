@admin @priority=P3
Feature: Admin Reset
  As a developer using gmock
  I want to reset all server state via the admin API
  So that I can start fresh between tests

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Reset clears all stubs
    Given I register a stub via admin API for GET /api/reset-me returning 200
    When I reset via admin API
    Then the response status is 200
    When I send a GET request to "/api/reset-me"
    Then the response status is 404

  @admin @priority=P3
  Scenario: Reset clears request log
    Given I register a stub via admin API for GET /api/test returning 200
    When I send a GET request to "/api/test"
    When I reset via admin API
    When I list request log
    Then the response status is 200

  @admin @priority=P3
  Scenario: After reset previously working stub returns 404
    Given I register a stub via admin API for GET /api/working returning 200
    When I send a GET request to "/api/working"
    Then the response status is 200
    When I reset via admin API
    When I send a GET request to "/api/working"
    Then the response status is 404
