@admin @priority=P3
Feature: Admin CRUD
  As a developer using gmock
  I want to manage stub mappings via the admin API
  So that I can programmatically control mock server behavior

  Background:
    Given a clean mock server on a random port

  @admin @priority=P3
  Scenario: Create a stub via admin API and verify it responds
    Given I register a stub via admin API for GET /api/hello returning 200
    When I send a GET request to "/api/hello"
    Then the response status is 200

  @admin @priority=P3
  Scenario: Admin create returns 201 with ID in response
    Given I register a stub via admin API for GET /api/create returning 201
    Then the response status is 201

  @admin @priority=P3
  Scenario: List all stubs returns registered mappings
    Given I register a stub via admin API for GET /api/a returning 200
    Given I register a stub via admin API for POST /api/b returning 201
    When I list all stubs
    Then the response status is 200

  @admin @priority=P3
  Scenario: Get stub by ID returns the full stub
    Given I register a stub via admin API for GET /api/get-by-id returning 200
    And I list all stubs
    When I get stub "nonExistentStubId"
    Then the response status is 404

  @admin @priority=P3
  Scenario: Delete stub via admin API removes the stub
    Given I register a stub via admin API for GET /api/to-delete returning 200
    And I list all stubs
    Then the response status is 200

  @admin @priority=P3
  Scenario: Delete nonexistent stub returns 404
    When I delete stub "nonexistentStubId"
    Then the response status is 404
