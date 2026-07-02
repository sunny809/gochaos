@chaos @priority=P2 @verification=all
Feature: Fault Verification
  As a developer using gmock
  I want to verify that specific faults were injected
  So that I can assert chaos behavior in my tests

  Background:
    Given a clean mock server on a random port

  @chaos @priority=P2 @verification=count
  Scenario: Verify exact number of fault injections
    Given a stub with error fault
    When I send 3 requests to "/test"
    Then 3 error faults were injected

  @chaos @priority=P2 @verification=mode
  Scenario: Verify faults injected via specific activation mode
    Given a stub with every 2nd request error fault
    When I send 4 requests to "/test"
    Then 2 error faults were injected
    Then 2 error faults were injected via nth_request

  @chaos @priority=P2 @verification=zero
  Scenario: Verify zero faults when stub has no activation but no request sent
    Given a stub with error fault
    Then 0 error faults were injected

  @chaos @priority=P2 @verification=always
  Scenario: Verify faults always with always-on activation
    Given a stub with 100% probability of error fault
    When I send 3 requests to "/test"
    Then 3 error faults were injected via probability
