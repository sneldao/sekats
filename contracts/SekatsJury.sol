// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title SekatsJury
/// @notice Permissionless juror verdicts on agent/shadow-LLM disagreements.
/// Anyone may record which side of a disagreement they endorse; the record
/// is append-only, self-custodied (msg.sender is the juror), and jurors may
/// change their verdict — counts move with them. This is a public review
/// surface, deliberately separate from the gated SekatsAttestations
/// registry: observing is open, promoting policy is not.
contract SekatsJury {
    /// @dev verdict encoding: 0 = none, 1 = side with agent, 2 = side with shadow reviewer
    mapping(bytes32 => mapping(address => uint8)) public verdictOf;
    mapping(bytes32 => uint256) public agentSide;
    mapping(bytes32 => uint256) public shadowSide;

    event JurorVerdict(
        bytes32 indexed decisionKey,
        address indexed juror,
        uint8 verdict
    );

    function attest(bytes32 decisionKey, uint8 verdict) external {
        require(decisionKey != bytes32(0), "zero key");
        require(verdict == 1 || verdict == 2, "invalid verdict");
        uint8 prev = verdictOf[decisionKey][msg.sender];
        if (prev == verdict) return;
        if (prev == 1) agentSide[decisionKey]--;
        if (prev == 2) shadowSide[decisionKey]--;
        verdictOf[decisionKey][msg.sender] = verdict;
        if (verdict == 1) agentSide[decisionKey]++;
        if (verdict == 2) shadowSide[decisionKey]++;
        emit JurorVerdict(decisionKey, msg.sender, verdict);
    }

    function tally(bytes32 decisionKey) external view returns (uint256 agent, uint256 shadow) {
        return (agentSide[decisionKey], shadowSide[decisionKey]);
    }
}
