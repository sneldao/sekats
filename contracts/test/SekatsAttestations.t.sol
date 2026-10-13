// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "../SekatsAttestations.sol";

// Self-contained Foundry tests, no forge-std dependency or network access.
interface Vm {
    function prank(address sender) external;
    function chainId(uint256 chainId) external;
    function etch(address target, bytes calldata code) external;
    function expectRevert(bytes calldata reason) external;
    function expectEmit(bool, bool, bool, bool) external;
}

contract SekatsAttestationsTest {
    Vm constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));
    SekatsAttestations registry;
    address approver;
    bytes32 constant KEY = bytes32(uint256(1));
    bytes32 constant DOC = bytes32(uint256(2));
    // Fixed public signature components, not a private key. Each test authorizes
    // the recovered signer for this domain. No wallet or secret is needed.
    bytes32 constant R = 0x79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798;
    event Anchored(bytes32 indexed draftKey, bytes32 indexed docHash, address indexed attester, address relayer);
    event PolicyAnchored(bytes32 indexed draftKey, bytes32 indexed policyHash, uint256 index, address indexed attester);

    function setUp() public {
        registry = new SekatsAttestations(address(this));
        approver = ecrecover(registry.approvalDigest(KEY, DOC), 27, R, bytes32(uint256(1)));
        require(approver != address(0), "invalid fixture");
        registry.setApprover(approver, true);
    }

    function signature(uint256 fixture) internal pure returns (bytes memory) {
        return abi.encodePacked(R, bytes32(fixture), uint8(27));
    }

    function testValidSignatureRecordsSignerAndRelayer() public {
        bytes memory sig = signature(1);
        vm.expectEmit(true, true, true, true);
        emit Anchored(KEY, DOC, approver, address(this));
        registry.anchorWithSig(KEY, DOC, sig);
        require(registry.anchored(KEY) == DOC, "wrong hash");
        require(registry.attesters(KEY) == approver, "relayer recorded as signer");
    }

    function testGoAndEthersDigestVector() public {
        address vectorAddress = 0x1111111111111111111111111111111111111111;
        vm.etch(vectorAddress, address(registry).code);
        vm.chainId(143);
        bytes32 digest = SekatsAttestations(vectorAddress).approvalDigest(
            sha256(bytes("sekats/draft/vector")), sha256(bytes("canonical document"))
        );
        require(digest == 0xf49fe2542718ca572b676f3e0c3525b4b19fd13696f56cbe683f5583feea3aa0, "digest mismatch");
    }

    function testNonApproverCannotSquat() public {
        bytes memory sig = signature(2);
        vm.expectRevert(bytes("not approver"));
        registry.anchorWithSig(KEY, DOC, sig);
        require(registry.anchored(KEY) == bytes32(0), "squatted");
    }

    function testChangedDocumentRejected() public {
        vm.expectRevert(bytes("not approver"));
        registry.anchorWithSig(KEY, bytes32(uint256(999)), signature(1));
    }

    function testHighSAndInvalidRecoveryRejected() public {
        bytes memory highS = abi.encodePacked(
            R, bytes32(uint256(0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140)),
            uint8(28)
        );
        vm.expectRevert(bytes("invalid signature"));
        registry.anchorWithSig(KEY, DOC, highS);
        vm.expectRevert(bytes("invalid signature"));
        registry.anchorWithSig(KEY, DOC, abi.encodePacked(R, bytes32(uint256(1)), uint8(2)));
    }

    function testRecoveryZeroAccepted() public {
        registry.anchorWithSig(KEY, DOC, abi.encodePacked(R, bytes32(uint256(1)), uint8(0)));
        require(registry.attesters(KEY) == approver, "wrong signer");
    }

    function testZeroPolicyHashRejected() public {
        registry.anchorWithSig(KEY, DOC, signature(1));
        vm.expectRevert(bytes("zero policy hash"));
        registry.anchorPolicy(KEY, bytes32(0));
    }

    function testRevokedApproverRejected() public {
        bytes memory sig = signature(1);
        registry.setApprover(approver, false);
        vm.expectRevert(bytes("not approver"));
        registry.anchorWithSig(KEY, DOC, sig);
    }

    function testReplayAnotherChainRejected() public {
        bytes memory sig = signature(1);
        vm.chainId(block.chainid + 1);
        vm.expectRevert(bytes("not approver"));
        registry.anchorWithSig(KEY, DOC, sig);
    }

    function testReplayAnotherContractRejected() public {
        bytes memory sig = signature(1);
        SekatsAttestations other = new SekatsAttestations(address(this));
        other.setApprover(approver, true);
        vm.expectRevert(bytes("not approver"));
        other.anchorWithSig(KEY, DOC, sig);
    }

    function testDoubleAnchorRejected() public {
        bytes memory sig = signature(1);
        registry.anchorWithSig(KEY, DOC, sig);
        vm.expectRevert(bytes("already anchored"));
        registry.anchorWithSig(KEY, DOC, sig);
    }

    function testPolicyAnchoringOrdering() public {
        vm.expectRevert(bytes("draft not anchored"));
        registry.anchorPolicy(KEY, bytes32(uint256(3)));
        registry.anchorWithSig(KEY, DOC, signature(1));
        registry.anchorPolicy(KEY, bytes32(uint256(3)));
        vm.expectEmit(true, true, true, true);
        emit PolicyAnchored(KEY, bytes32(uint256(4)), 1, approver);
        vm.prank(approver);
        registry.anchorPolicy(KEY, bytes32(uint256(4)));
        require(registry.policyCount(KEY) == 2, "wrong length");
        require(registry.policyHistory(KEY, 0) == bytes32(uint256(3)), "overwritten");
        require(registry.policyHistory(KEY, 1) == bytes32(uint256(4)), "wrong order");
    }

    function testPolicyAndApproverAdministrationRestricted() public {
        vm.prank(address(123));
        vm.expectRevert(bytes("not owner"));
        registry.setApprover(address(123), true);
        vm.prank(address(123));
        vm.expectRevert(bytes("not authorized"));
        registry.anchorPolicy(KEY, DOC);
    }

    function testMalformedAndZeroHashesRejected() public {
        vm.expectRevert(bytes("invalid signature length"));
        registry.anchorWithSig(KEY, DOC, hex"00");
        vm.expectRevert(bytes("zero hash"));
        registry.anchorWithSig(KEY, bytes32(0), hex"00");
        vm.expectRevert(bytes("zero owner"));
        new SekatsAttestations(address(0));
    }
}
