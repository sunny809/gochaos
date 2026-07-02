@chaos @priority=P2 @fault=all
Feature: Fault Injection
  As a developer using gmock
  I want to simulate various network-level faults
  So that I can test how my application handles failures

  Background:
    Given a clean mock server on a random port

  @chaos @priority=P2 @fault=error @idempotent
  Scenario: Error fault closes connection immediately
    Given a stub with error fault
    When I send 1 requests to "/test"
    Then 1 error faults were injected

  @chaos @priority=P2 @fault=empty @http-observable
  Scenario: Empty fault returns response with empty body
    Given a stub with empty fault
    When I send 1 requests to "/test"
    Then 1 empty faults were injected

  @chaos @priority=P2 @fault=connection_reset
  Scenario: Connection reset fault terminates connection abruptly
    Given a stub with connection_reset fault
    When I send 1 requests to "/test"
    Then 1 connection_reset faults were injected

  @chaos @priority=P2 @fault=malformed
  Scenario: Malformed fault sends garbage response
    Given a stub with malformed fault
    When I send 1 requests to "/test"
    Then 1 malformed faults were injected

  @chaos @priority=P2 @fault=random_data
  Scenario: Random data fault sends specified number of random bytes
    Given a stub with random_data fault and 512 random data bytes
    When I send 1 requests to "/test"
    Then 1 random_data faults were injected

  @chaos @priority=P2 @fault=slow_close
  Scenario: Slow close fault delays connection close after response
    Given a stub with slow_close fault
    When I send 1 requests to "/test"
    Then 1 slow_close faults were injected

  @chaos @priority=P2 @fault=rate_limit
  Scenario: Rate limit fault returns 429 after burst threshold
    Given a stub with rate_limit fault with 5 per second
    When I send 10 requests to "/test"
    Then at most 6 responses are 429
