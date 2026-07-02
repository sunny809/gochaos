@chaos @priority=P2 @activation=all
Feature: Activation Modes
  As a developer using gmock
  I want to control when faults are triggered using activation modes
  So that I can simulate realistic fault scenarios

  Background:
    Given a clean mock server on a random port

  @chaos @priority=P2 @activation=probability
  Scenario: Probability activation fires faults with specified likelihood
    Given a stub with 50% probability of error fault
    When I send 10 requests to "/test"
    Then at least 2 error faults were injected
    Then at most 8 error faults were injected

  @chaos @priority=P2 @activation=everyNthRequest
  Scenario: Every Nth request activation fires fault on every Nth request
    Given a stub with every 3rd request error fault
    When I send 9 requests to "/test"
    Then 3 error faults were injected

  @chaos @priority=P2 @activation=activeBetween
  Scenario: Active between activation fires fault within a time window
    Given a stub with error fault active between 0ms and 60000ms
    When I send 5 requests to "/test"
    Then 5 error faults were injected

  @chaos @priority=P2 @activation=combined
  Scenario: Combined probability and every Nth request
    Given a stub with 100% probability of error fault
    When I send 5 requests to "/test"
    Then 5 error faults were injected
