@core @priority=P1
Feature: Stub Lifecycle
  As a developer using gmock
  I want to register, match, list, and remove stub definitions
  So that I can control mock server behavior in my tests

  Background:
    Given a clean mock server on a random port

  Scenario: Register a GET stub and receive a matched response
    Given a stub for GET /api/hello returns 200
    When I send a GET request to "/api/hello"
    Then the response status is 200

  Scenario: Register a stub with a response body
    Given a stub for GET /api/message returns 200 with body "Hello, World!"
    When I send a GET request to "/api/message"
    Then the response status is 200
    And the response body is "Hello, World!"

  Scenario: Unmatched request returns 404
    When I send a GET request to "/api/unknown"
    Then the response status is 404

  Scenario: Register a POST stub and verify method matching
    Given a stub for POST /api/data returns 201
    When I send a POST request to "/api/data"
    Then the response status is 201

  Scenario: Method mismatch returns 404
    Given a stub for POST /api/data returns 201
    When I send a GET request to "/api/data"
    Then the response status is 404

  Scenario: Delete a stub and confirm it no longer matches
    Given a stub for GET /api/temp returns 200
    When I delete the stub
    And I send a GET request to "/api/temp"
    Then the response status is 404

  Scenario: Reset server clears all stubs
    Given a stub for GET /api/reset-test returns 200
    When I reset the server
    And I send a GET request to "/api/reset-test"
    Then the response status is 404
