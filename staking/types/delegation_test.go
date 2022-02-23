package types

import (
	"math/big"
	"testing"

	common "github.com/ethereum/go-ethereum/common"
	common2 "github.com/harmony-one/harmony/internal/common"
)

var (
	testAddr, _   = common2.Bech32ToAddress("one129r9pj3sk0re76f7zs3qz92rggmdgjhtwge62k")
	delegatorAddr = common.Address(testAddr)
	delegationAmt = big.NewInt(100000)
	// create a new delegation:
	delegation = NewDelegation(delegatorAddr, delegationAmt)
)

func TestUndelegate(t *testing.T) {
	epoch1 := big.NewInt(10)
	amount1 := big.NewInt(1000)
	delegation.Undelegate(epoch1, amount1, nil)

	// check the undelegation's Amount
	if delegation.Undelegations[0].Amount.Cmp(amount1) != 0 {
		t.Errorf("undelegate failed, amount does not match")
	}
	// check the undelegation's Epoch
	if delegation.Undelegations[0].Epoch.Cmp(epoch1) != 0 {
		t.Errorf("undelegate failed, epoch does not match")
	}

	epoch2 := big.NewInt(12)
	amount2 := big.NewInt(2000)
	delegation.Undelegate(epoch2, amount2, nil)

	// check the number of undelegations
	if len(delegation.Undelegations) != 2 {
		t.Errorf("total number of undelegations should have been two")
	}
}

func TestTotalInUndelegation(t *testing.T) {
	var totalAmount = delegation.TotalInUndelegation()

	// check the total amount of undelegation
	if totalAmount.Cmp(big.NewInt(3000)) != 0 {
		t.Errorf("total undelegation amount is not correct")
	}
}

func TestDeleteEntry(t *testing.T) {
	// add the third delegation
	// Undelegations[]: 1000, 2000, 3000
	epoch3 := big.NewInt(15)
	amount3 := big.NewInt(3000)
	delegation.Undelegate(epoch3, amount3, nil)

	// delete the second undelegation entry
	// Undelegations[]: 1000, 3000
	deleteEpoch := big.NewInt(12)
	delegation.DeleteEntry(deleteEpoch)

	// check if the Undelegtaions[1] == 3000
	if delegation.Undelegations[1].Amount.Cmp(big.NewInt(3000)) != 0 {
		t.Errorf("deleting an undelegation entry fails, amount is not correct")
	}
}

func TestUnlockedLastEpochInCommittee(t *testing.T) {
	lastEpochInCommittee := big.NewInt(17)
	curEpoch := big.NewInt(24)

	epoch4 := big.NewInt(21)
	amount4 := big.NewInt(4000)
	delegation.Undelegate(epoch4, amount4, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, false)
	if result.Cmp(big.NewInt(8000)) != 0 {
		t.Errorf("removing an unlocked undelegation fails")
	}
}

func TestUnlockedLastEpochInCommitteeFail(t *testing.T) {
	delegation := NewDelegation(delegatorAddr, delegationAmt)
	lastEpochInCommittee := big.NewInt(18)
	curEpoch := big.NewInt(24)

	epoch4 := big.NewInt(21)
	amount4 := big.NewInt(4000)
	delegation.Undelegate(epoch4, amount4, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, false)
	if result.Cmp(big.NewInt(0)) != 0 {
		t.Errorf("premature delegation shouldn't be unlocked")
	}
}

func TestUnlockedFullPeriod(t *testing.T) {
	lastEpochInCommittee := big.NewInt(34)
	curEpoch := big.NewInt(34)

	epoch5 := big.NewInt(27)
	amount5 := big.NewInt(4000)
	delegation.Undelegate(epoch5, amount5, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, false)
	if result.Cmp(big.NewInt(4000)) != 0 {
		t.Errorf("removing an unlocked undelegation fails")
	}
}

func TestQuickUnlock(t *testing.T) {
	lastEpochInCommittee := big.NewInt(44)
	curEpoch := big.NewInt(44)

	epoch7 := big.NewInt(44)
	amount7 := big.NewInt(4000)
	delegation.Undelegate(epoch7, amount7, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 0, false)
	if result.Cmp(big.NewInt(4000)) != 0 {
		t.Errorf("removing an unlocked undelegation fails")
	}
}

func TestUnlockedFullPeriodFail(t *testing.T) {
	delegation := NewDelegation(delegatorAddr, delegationAmt)
	lastEpochInCommittee := big.NewInt(34)
	curEpoch := big.NewInt(34)

	epoch5 := big.NewInt(28)
	amount5 := big.NewInt(4000)
	delegation.Undelegate(epoch5, amount5, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, false)
	if result.Cmp(big.NewInt(0)) != 0 {
		t.Errorf("premature delegation shouldn't be unlocked")
	}
}

func TestUnlockedPremature(t *testing.T) {
	lastEpochInCommittee := big.NewInt(44)
	curEpoch := big.NewInt(44)

	epoch6 := big.NewInt(42)
	amount6 := big.NewInt(4000)
	delegation.Undelegate(epoch6, amount6, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, false)
	if result.Cmp(big.NewInt(0)) != 0 {
		t.Errorf("premature delegation shouldn't be unlocked")
	}
}

func TestNoEarlyUnlock(t *testing.T) {
	lastEpochInCommittee := big.NewInt(17)
	curEpoch := big.NewInt(24)

	epoch4 := big.NewInt(21)
	amount4 := big.NewInt(4000)
	delegation.Undelegate(epoch4, amount4, nil)

	result := delegation.RemoveUnlockedUndelegations(curEpoch, lastEpochInCommittee, 7, true)
	if result.Cmp(big.NewInt(0)) != 0 {
		t.Errorf("should not allow early unlock")
	}
}

func TestMinRemainingDelegation(t *testing.T) {
	// make it again so that the test is idempotent
	delegation = NewDelegation(delegatorAddr, big.NewInt(100000))
	minimumAmount := big.NewInt(50000) // half of the delegation amount
	// first undelegate such that remaining < minimum
	epoch := big.NewInt(10)
	amount := big.NewInt(50001)
	expect := "Minimum: 50000, Remaining: 49999: remaining delegation must be 0 or >= 100 ONE"
	if err := delegation.Undelegate(epoch, amount, minimumAmount); err == nil || err.Error() != expect {
		t.Errorf("Expected error %v but got %v", expect, err)
	}

	// then undelegate such that remaining >= minimum
	amount = big.NewInt(50000)
	epoch = big.NewInt(11)
	if err := delegation.Undelegate(epoch, amount, minimumAmount); err != nil {
		t.Errorf("Expected no error but got %v", err)
	}
	if len(delegation.Undelegations) != 1 {
		t.Errorf("Unexpected length %d", len(delegation.Undelegations))
	}
	if delegation.Amount.Cmp(minimumAmount) != 0 {
		t.Errorf("Unexpected delegation.Amount %d; minimumAmount %d",
			delegation.Amount,
			minimumAmount,
		)
	}

	// finally delegate such that remaining is zero
	epoch = big.NewInt(12)
	if err := delegation.Undelegate(epoch, delegation.Amount, minimumAmount); err != nil {
		t.Errorf("Expected no error but got %v", err)
	}
	if len(delegation.Undelegations) != 2 { // separate epoch
		t.Errorf("Unexpected length %d", len(delegation.Undelegations))
	}
	if delegation.Amount.Cmp(common.Big0) != 0 {
		t.Errorf("Unexpected delegation.Amount %d; minimumAmount %d",
			delegation.Amount,
			common.Big0,
		)
	}
}
