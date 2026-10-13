// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title SekatsAttestations
/// @notice First signed approval wins; policy lineage is append-only.
/// SHA-256 is used for document/file hashes, Keccak-256 for EIP-712.
contract SekatsAttestations {
    address public immutable owner;
    mapping(address => bool) public isApprover;
    mapping(bytes32 => bytes32) public anchored;
    mapping(bytes32 => address) public attesters;
    mapping(bytes32 => bytes32[]) public policyHistory;

    bytes32 public constant APPROVAL_TYPEHASH =
        keccak256("Approval(bytes32 draftKey,bytes32 docHash)");
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    uint256 private constant HALF_ORDER =
        0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    event ApproverSet(address indexed approver, bool allowed);
    event Anchored(
        bytes32 indexed draftKey,
        bytes32 indexed docHash,
        address indexed attester,
        address relayer
    );
    event PolicyAnchored(
        bytes32 indexed draftKey,
        bytes32 indexed policyHash,
        uint256 index,
        address indexed attester
    );

    constructor(address initialOwner) {
        require(initialOwner != address(0), "zero owner");
        owner = initialOwner;
    }

    function setApprover(address approver, bool allowed) external {
        require(msg.sender == owner, "not owner");
        require(approver != address(0), "zero approver");
        isApprover[approver] = allowed;
        emit ApproverSet(approver, allowed);
    }

    /// @dev Recomputed to bind signatures to the current chain, including forks.
    function domainSeparator() public view returns (bytes32) {
        return keccak256(abi.encode(
            DOMAIN_TYPEHASH, keccak256("SekatsAttestations"), keccak256("2"),
            block.chainid, address(this)
        ));
    }

    function approvalDigest(bytes32 draftKey, bytes32 docHash) public view returns (bytes32) {
        return keccak256(abi.encodePacked(
            "\x19\x01", domainSeparator(),
            keccak256(abi.encode(APPROVAL_TYPEHASH, draftKey, docHash))
        ));
    }

    /// @notice Anyone may relay, but only a currently authorized signer may approve.
    function anchorWithSig(bytes32 draftKey, bytes32 docHash, bytes calldata signature) external {
        require(draftKey != bytes32(0) && docHash != bytes32(0), "zero hash");
        require(anchored[draftKey] == bytes32(0), "already anchored");
        require(signature.length == 65, "invalid signature length");
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }
        if (v < 27) v += 27;
        require((v == 27 || v == 28) && uint256(s) <= HALF_ORDER, "invalid signature");
        address signer = ecrecover(approvalDigest(draftKey, docHash), v, r, s);
        require(signer != address(0) && isApprover[signer], "not approver");
        anchored[draftKey] = docHash;
        attesters[draftKey] = signer;
        emit Anchored(draftKey, docHash, signer, msg.sender);
    }

    /// @notice An authorized operator attests to exact resulting policy-file bytes.
    /// This is lineage, not proof that an agent executes those bytes.
    function anchorPolicy(bytes32 draftKey, bytes32 policyHash) external {
        require(msg.sender == owner || isApprover[msg.sender], "not authorized");
        require(anchored[draftKey] != bytes32(0), "draft not anchored");
        require(policyHash != bytes32(0), "zero policy hash");
        uint256 index = policyHistory[draftKey].length;
        policyHistory[draftKey].push(policyHash);
        emit PolicyAnchored(draftKey, policyHash, index, msg.sender);
    }

    function policyCount(bytes32 draftKey) external view returns (uint256) {
        return policyHistory[draftKey].length;
    }
}
