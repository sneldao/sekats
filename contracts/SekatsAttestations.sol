// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title SekatsAttestations
/// @notice Append-only registry anchoring human-approved policy edits
///         ("attestations") produced by the sekats console.
///
/// The console builds a canonical JSON document describing an approved
/// policy draft, takes SHA-256 over those bytes, and calls
/// `anchor(draftKey, docHash)`. `draftKey` is
/// `SHA-256("sekats/draft/" || draftID)`; `docHash` is the doc digest.
/// Anyone can verify an attestation offline: fetch the canonical doc
/// from the console, hash it, and compare against `anchored[draftKey]`.
///
/// One anchor per draft — re-anchoring the same key reverts, so the
/// first human approval is the one that counts.
contract SekatsAttestations {
    event Anchored(
        bytes32 indexed draftKey,
        bytes32 indexed docHash,
        address indexed attester
    );

    /// @notice draftKey => SHA-256 of the canonical attestation document.
    mapping(bytes32 => bytes32) public anchored;

    /// @param draftKey SHA-256("sekats/draft/" || draftID)
    /// @param docHash  SHA-256 of the canonical attestation document
    function anchor(bytes32 draftKey, bytes32 docHash) external {
        require(anchored[draftKey] == bytes32(0), "already anchored");
        anchored[draftKey] = docHash;
        emit Anchored(draftKey, docHash, msg.sender);
    }
}
